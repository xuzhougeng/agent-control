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
	router    skills.Router
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

// SetRouter wires automatic per-turn skill routing. When set, every user
// turn is first sent through the router, and a matched skill's prompt is
// woven into the persisted user message via skills.WrapUserInput. The
// agent's req.System is NEVER modified by routing — that would invalidate
// the provider's prefix cache. nil router == no automatic routing.
func (a *Agent) SetRouter(r skills.Router) { a.router = r }

// Skills returns the registered skill registry (may be nil if not configured).
func (a *Agent) Skills() *skills.Registry { return a.skills }

// Tools returns the tool registry the agent dispatches against.
func (a *Agent) Tools() *tools.Registry { return a.tools }

// Rewind drops the most recent n user-anchored turns from the session: the
// last n user messages plus every assistant / tool message that followed
// them. Returns (dropped messages, error). Used by the REPL's :rewind to
// give the operator Claude-Code-style "undo my last question(s)".
//
// If the session has fewer than n user turns, returns an error and leaves
// memory untouched. n must be >= 1.
func (a *Agent) Rewind(ctx context.Context, sessionID string, n int) (int, error) {
	if n < 1 {
		return 0, errors.New("rewind: n must be >= 1")
	}
	hist, err := a.mem.LoadMessages(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	userIdx := make([]int, 0)
	for i, m := range hist {
		if m.Role == string(llm.RoleUser) {
			userIdx = append(userIdx, i)
		}
	}
	if len(userIdx) < n {
		return 0, fmt.Errorf("rewind: only %d user turn(s) in session, asked to drop %d", len(userIdx), n)
	}
	cutAt := userIdx[len(userIdx)-n]
	var cutoffID int64
	if cutAt > 0 {
		cutoffID = hist[cutAt-1].ID
	}
	return a.mem.DropAfter(ctx, sessionID, cutoffID)
}

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
	persistedInput := a.applyRouter(ctx, userInput, emit)
	return a.runTurnBody(ctx, sessionID, persistedInput, emit)
}

// applyRouter runs the auto-router (if configured) and returns the user
// input wrapped with the picked skill's prompt, or the original input when
// no router is configured or no skill matched. emit may surface router
// errors as agent events.
//
// We must NEVER touch a.opts.SystemPrompt here; mutating req.System would
// invalidate the provider's prefix cache.
func (a *Agent) applyRouter(ctx context.Context, userInput string, emit func(Event)) string {
	if a.router == nil || a.skills == nil {
		return userInput
	}
	all := a.skillsSlice()
	if len(all) == 0 {
		return userInput
	}
	name, err := a.router.Route(ctx, userInput, all)
	switch {
	case err != nil:
		emit(Event{Kind: EventError, Text: fmt.Sprintf("router: %v (continuing without skill)", err)})
		return userInput
	case name == "":
		return userInput
	}
	sk, ok := a.skills.Get(name)
	if !ok {
		return userInput
	}
	wrapped := skills.WrapUserInput(userInput, sk)
	if wrapped == userInput {
		return userInput
	}
	emit(Event{Kind: EventRouter, Text: name})
	return wrapped
}

// RunWithForcedSkill drives one turn pinned to skill — bypassing the
// auto-router. The wrapped user message is byte-identical to what a
// router-picked turn would produce (same skills.WrapUserInput), so memory
// replay and prefix caches behave the same.
//
// A nil skill is equivalent to RunWithListener (no wrap, no route).
//
// Surfaces a "pinned" EventRouter so transcripts make clear the skill was
// operator-chosen, not auto-picked.
func (a *Agent) RunWithForcedSkill(ctx context.Context, sessionID, userInput string, skill *skills.Skill, listener EventListener) (string, error) {
	if skill == nil {
		return a.RunWithListener(ctx, sessionID, userInput, listener)
	}
	emit := func(e Event) {
		if listener != nil {
			listener(e)
		}
	}
	if err := a.ensureSession(ctx, sessionID); err != nil {
		return "", err
	}
	persistedInput := skills.WrapUserInput(userInput, skill)
	if persistedInput != userInput {
		emit(Event{Kind: EventRouter, Text: skill.Name + " (pinned)"})
	}
	return a.runTurnBody(ctx, sessionID, persistedInput, emit)
}

// runTurnBody is the post-routing body shared by RunWithListener and
// RunWithForcedSkill: persist the user message, then loop tool calls until
// the model stops or MaxIterations is hit.
func (a *Agent) runTurnBody(ctx context.Context, sessionID, persistedInput string, emit func(Event)) (string, error) {
	userMsg := memory.Message{
		Role: string(llm.RoleUser), Content: persistedInput, At: time.Now().UnixMilli(),
	}
	if err := a.mem.AppendMessage(ctx, sessionID, userMsg); err != nil {
		return "", err
	}
	emit(Event{Kind: EventUser, Text: persistedInput})

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
		// Once an assistant message with tool_calls is in memory, every call
		// MUST get a tool result message before we exit this iteration —
		// otherwise the next provider request sees malformed history and
		// OpenAI rejects with "tool_calls must be followed by tool messages".
		// Track outstanding ones; flush synthetic results for any left over.
		pending := make(map[string]bool, len(resp.Message.ToolCalls))
		for _, tc := range resp.Message.ToolCalls {
			pending[tc.ID] = true
		}
		flushPending := func(reason string) {
			for id := range pending {
				result := memory.Message{
					Role:       string(llm.RoleTool),
					ToolUseID:  id,
					ToolResult: reason,
					ToolError:  true,
					At:         time.Now().UnixMilli(),
				}
				// Detached context so a canceled run still records the
				// result. The assistant message is already persisted; this
				// keeps the pair intact.
				_ = a.mem.AppendMessage(context.Background(), sessionID, result)
				emit(Event{Kind: EventToolResult, ToolID: id, Text: reason, IsError: true})
				delete(pending, id)
			}
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
			// Detached context: ctx may be canceled, but the tool already
			// produced its result and we owe it to the assistant turn to
			// record one.
			if err := a.mem.AppendMessage(context.Background(), sessionID, result); err != nil {
				flushPending(fmt.Sprintf("memory write failed: %v", err))
				return final, err
			}
			delete(pending, tc.ID)
			emit(Event{Kind: EventToolResult, ToolID: tc.ID, Text: out, IsError: isErr})
			if ctx.Err() != nil {
				flushPending(fmt.Sprintf("canceled by operator: %v", ctx.Err()))
				return final, ctx.Err()
			}
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

// skillsSlice returns all loaded skills in name-sorted order. Sorted output
// matters for the router: it makes the system prompt deterministic across
// process restarts, which keeps the router's prefix cache warm.
func (a *Agent) skillsSlice() []*skills.Skill {
	if a.skills == nil {
		return nil
	}
	names := a.skills.Names() // already sorted
	out := make([]*skills.Skill, 0, len(names))
	for _, n := range names {
		if s, ok := a.skills.Get(n); ok {
			out = append(out, s)
		}
	}
	return out
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
	return repairToolCallPairs(out)
}

// repairToolCallPairs guarantees every assistant tool_call is followed by a
// matching tool result message. If a prior run was hard-killed between
// recording the assistant turn and executing the tool, the persisted history
// is missing the tool result; OpenAI rejects such requests with "An assistant
// message with 'tool_calls' must be followed by tool messages". We splice in
// a synthetic "(canceled)" result so the conversation is replay-safe.
func repairToolCallPairs(in []llm.Message) []llm.Message {
	have := make(map[string]bool)
	for _, m := range in {
		if m.Role == llm.RoleTool && m.ToolUseID != "" {
			have[m.ToolUseID] = true
		}
	}
	missing := false
	for _, m := range in {
		if m.Role != llm.RoleAssistant {
			continue
		}
		for _, tc := range m.ToolCalls {
			if !have[tc.ID] {
				missing = true
				break
			}
		}
		if missing {
			break
		}
	}
	if !missing {
		return in
	}
	out := make([]llm.Message, 0, len(in)+2)
	for _, m := range in {
		out = append(out, m)
		if m.Role != llm.RoleAssistant || len(m.ToolCalls) == 0 {
			continue
		}
		for _, tc := range m.ToolCalls {
			if have[tc.ID] {
				continue
			}
			out = append(out, llm.Message{
				Role:       llm.RoleTool,
				ToolUseID:  tc.ID,
				ToolResult: "(canceled — no result recorded for prior run)",
				ToolError:  true,
			})
			have[tc.ID] = true
		}
	}
	return out
}
