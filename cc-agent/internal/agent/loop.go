// Package agent runs the ReAct-style main loop: it forwards a user turn to
// the LLM, executes any tool calls the model emits, feeds results back, and
// stops when the model emits StopEnd or the iteration cap is reached.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cc-agent/internal/llm"
	"cc-agent/internal/memory"
	"cc-agent/internal/skills"
	"cc-agent/internal/tools"
)

type Options struct {
	Model         string
	MaxIterations int
	MaxTokens     int
	SystemPrompt  string
}

// Agent is a stateless dispatcher. Per-session conversation state lives in the
// Memory store, keyed by session id. Optional Skills + Reflector enable the
// :reflect command which distills a session into a reusable skill JSON.
type Agent struct {
	provider  llm.Provider
	tools     *tools.Registry
	mem       memory.Store
	opts      Options
	listener  EventListener
	skills    *skills.Registry
	skillsDir string
	reflector skills.Reflector
}

// EventListener is invoked at every step boundary so callers (CLI, HTTP, WS)
// can stream progress.
type EventListener func(Event)

func New(p llm.Provider, reg *tools.Registry, mem memory.Store, opts Options) *Agent {
	if opts.MaxIterations <= 0 {
		opts.MaxIterations = 12
	}
	if opts.Model == "" {
		opts.Model = "claude-sonnet-4-6"
	}
	return &Agent{provider: p, tools: reg, mem: mem, opts: opts}
}

// SetListener wires a streaming event sink. Set once at startup; not
// goroutine-safe.
func (a *Agent) SetListener(fn EventListener) { a.listener = fn }

// SetSkills wires the skill registry, on-disk save dir, and reflector.
// All three must be set for Reflect() to succeed.
func (a *Agent) SetSkills(reg *skills.Registry, dir string, r skills.Reflector) {
	a.skills = reg
	a.skillsDir = dir
	a.reflector = r
}

// Skills returns the registered skill registry (may be nil if not configured).
func (a *Agent) Skills() *skills.Registry { return a.skills }

// Tools returns the tool registry the agent dispatches against.
func (a *Agent) Tools() *tools.Registry { return a.tools }

// Reflect distills the current session's transcript into a skill JSON,
// writes it to <skills_dir>/<name>.json, and loads it into the registry.
// description is optional (the reflector will write one if empty).
func (a *Agent) Reflect(ctx context.Context, sessionID, name, description string) (*skills.Skill, string, error) {
	if a.reflector == nil {
		return nil, "", errors.New("reflect: not configured (need provider + skills_dir)")
	}
	hist, err := a.mem.LoadMessages(ctx, sessionID)
	if err != nil {
		return nil, "", fmt.Errorf("reflect: load history: %w", err)
	}
	in := skills.DistillInput{
		Name: name, Description: description,
		History: toTranscript(hist),
	}
	skill, err := a.reflector.Distill(ctx, in)
	if err != nil {
		return nil, "", err
	}
	path, err := a.skills.Save(skill, a.skillsDir)
	if err != nil {
		return nil, "", fmt.Errorf("reflect: save: %w", err)
	}
	return skill, path, nil
}

// toTranscript flattens persisted memory.Message records into the role/text
// pairs the skills package consumes. Tool-call records become an
// ASSISTANT tool_use line plus the tool result line on the next entry.
func toTranscript(hist []memory.Message) []skills.TranscriptItem {
	out := make([]skills.TranscriptItem, 0, len(hist))
	for _, m := range hist {
		switch m.Role {
		case "user":
			out = append(out, skills.TranscriptItem{Role: "user", Text: m.Content})
		case "assistant":
			if m.Content != "" {
				out = append(out, skills.TranscriptItem{Role: "assistant", Text: m.Content})
			}
			if m.ToolCalls != "" {
				var calls []llm.ToolCall
				if err := json.Unmarshal([]byte(m.ToolCalls), &calls); err == nil {
					for _, tc := range calls {
						args := summarizeToolInput(tc.Input)
						out = append(out, skills.TranscriptItem{
							Role: "assistant", ToolName: tc.Name, ToolArgs: args,
						})
					}
				}
			}
		case "tool":
			out = append(out, skills.TranscriptItem{Role: "tool", Text: m.ToolResult})
		}
	}
	return out
}

func summarizeToolInput(in map[string]any) string {
	if len(in) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(in))
	for k, v := range in {
		s := fmt.Sprintf("%v", v)
		if len(s) > 80 {
			s = s[:80] + "…"
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, s))
	}
	return "{" + strings.Join(parts, " ") + "}"
}

// Run drives one user turn with the agent's configured default listener.
// Multi-tenant callers (cc-control bridge) should use RunWithListener so each
// turn streams to its own sink without racing.
func (a *Agent) Run(ctx context.Context, sessionID, userInput string) (string, error) {
	return a.RunWithListener(ctx, sessionID, userInput, a.listener)
}

// RunWithListener drives one user turn to completion. It appends the user
// message and any resulting assistant/tool messages to the session in mem,
// and returns the final assistant text content. The loop stops when the
// model emits StopEnd or MaxIterations is reached.
//
// listener may be nil (no streaming).
func (a *Agent) RunWithListener(ctx context.Context, sessionID, userInput string, listener EventListener) (string, error) {
	emit := func(e Event) {
		if listener != nil {
			listener(e)
		}
	}
	if err := a.ensureSession(ctx, sessionID); err != nil {
		return "", err
	}
	userMsg := memory.Message{
		Role: string(llm.RoleUser), Content: userInput, At: time.Now().UnixMilli(),
	}
	if err := a.mem.AppendMessage(ctx, sessionID, userMsg); err != nil {
		return "", err
	}
	emit(Event{Kind: EventUser, Text: userInput})

	defs := toolDefs(a.tools)
	final := ""

	for i := 0; i < a.opts.MaxIterations; i++ {
		hist, err := a.mem.LoadMessages(ctx, sessionID)
		if err != nil {
			return "", err
		}
		req := llm.Request{
			System:    a.opts.SystemPrompt,
			Messages:  toLLM(hist),
			Tools:     defs,
			Model:     a.opts.Model,
			MaxTokens: a.opts.MaxTokens,
		}
		resp, err := a.provider.Complete(ctx, req)
		if err != nil {
			emit(Event{Kind: EventError, Text: err.Error()})
			return "", err
		}
		assistant := memory.FromLLM(resp.Message)
		assistant.At = time.Now().UnixMilli()
		if err := a.mem.AppendMessage(ctx, sessionID, assistant); err != nil {
			return "", err
		}
		if resp.Message.Content != "" {
			emit(Event{Kind: EventAssistant, Text: resp.Message.Content})
			final = resp.Message.Content
		}
		if resp.StopReason != llm.StopToolUse || len(resp.Message.ToolCalls) == 0 {
			return final, nil
		}
		for _, tc := range resp.Message.ToolCalls {
			emit(Event{Kind: EventToolCall, ToolName: tc.Name, ToolInput: tc.Input, ToolID: tc.ID})
			out, isErr := a.runTool(ctx, tc)
			result := memory.Message{
				Role:       string(llm.RoleTool),
				ToolUseID:  tc.ID,
				ToolResult: out,
				ToolError:  isErr,
				At:         time.Now().UnixMilli(),
			}
			if err := a.mem.AppendMessage(ctx, sessionID, result); err != nil {
				return "", err
			}
			emit(Event{Kind: EventToolResult, ToolID: tc.ID, Text: out, IsError: isErr})
		}
	}
	return final, fmt.Errorf("max iterations reached (%d)", a.opts.MaxIterations)
}

func (a *Agent) runTool(ctx context.Context, tc llm.ToolCall) (string, bool) {
	t, ok := a.tools.Get(tc.Name)
	if !ok {
		return fmt.Sprintf("tool not registered: %s", tc.Name), true
	}
	out, err := t.Run(ctx, tc.Input)
	if err != nil {
		return fmt.Sprintf("error: %v\n%s", err, out), true
	}
	return out, false
}

func (a *Agent) ensureSession(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("session id required")
	}
	return a.mem.EnsureSession(ctx, id)
}

func toolDefs(r *tools.Registry) []llm.ToolDef {
	all := r.All()
	out := make([]llm.ToolDef, 0, len(all))
	for _, t := range all {
		out = append(out, llm.ToolDef{
			Name: t.Name(), Description: t.Description(), InputSchema: t.InputSchema(),
		})
	}
	return out
}

func toLLM(hist []memory.Message) []llm.Message {
	out := make([]llm.Message, 0, len(hist))
	for _, m := range hist {
		out = append(out, m.ToLLM())
	}
	return out
}
