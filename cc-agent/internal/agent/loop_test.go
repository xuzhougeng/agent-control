package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"cc-agent/internal/llm"
	"cc-agent/internal/memory"
	"cc-agent/internal/skills"
	"cc-agent/internal/tools"
)

// stubProvider returns a canned sequence of responses, advancing one per
// Complete call. Tests use it to script multi-turn tool-use flows without
// hitting a real API.
type stubProvider struct {
	responses []*llm.Response
	calls     int
}

func (s *stubProvider) Name() string { return "stub" }

func (s *stubProvider) Complete(_ context.Context, _ llm.Request) (*llm.Response, error) {
	if s.calls >= len(s.responses) {
		return nil, errors.New("no more canned responses")
	}
	r := s.responses[s.calls]
	s.calls++
	return r, nil
}

// fakeTool records its inputs and returns a fixed string.
type fakeTool struct {
	calls []map[string]any
}

func (*fakeTool) Name() string                    { return "echo" }
func (*fakeTool) Description() string             { return "echoes input" }
func (*fakeTool) InputSchema() map[string]any     { return map[string]any{"type": "object"} }
func (f *fakeTool) Run(_ context.Context, in map[string]any) (string, error) {
	f.calls = append(f.calls, in)
	return "echoed:" + in["msg"].(string), nil
}

func TestLoop_ToolUseThenStop(t *testing.T) {
	provider := &stubProvider{
		responses: []*llm.Response{
			{
				StopReason: llm.StopToolUse,
				Message: llm.Message{
					Role: llm.RoleAssistant,
					ToolCalls: []llm.ToolCall{{
						ID: "t1", Name: "echo", Input: map[string]any{"msg": "hi"},
					}},
				},
			},
			{
				StopReason: llm.StopEnd,
				Message:    llm.Message{Role: llm.RoleAssistant, Content: "done."},
			},
		},
	}
	reg := tools.NewRegistry()
	tool := &fakeTool{}
	reg.Register(tool)

	mem := memory.NewInMemory()
	a := New(provider, reg, mem, Options{Model: "m", MaxIterations: 5})

	out, err := a.Run(context.Background(), "s1", "please run echo")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "done." {
		t.Errorf("final = %q", out)
	}
	if len(tool.calls) != 1 || tool.calls[0]["msg"] != "hi" {
		t.Errorf("tool not called as expected: %+v", tool.calls)
	}
	hist, _ := mem.LoadMessages(context.Background(), "s1")
	if len(hist) != 4 {
		t.Errorf("expected user+assistant+tool+assistant=4 messages, got %d", len(hist))
	}
}

// TestRepairToolCallPairs covers the read-side defense: a prior run that died
// after recording an assistant tool_calls message but before the tool result
// must not produce a request that OpenAI rejects with "An assistant message
// with 'tool_calls' must be followed by tool messages".
func TestRepairToolCallPairs(t *testing.T) {
	in := []llm.Message{
		{Role: llm.RoleUser, Content: "make files"},
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{
				{ID: "call_a", Name: "bash", Input: map[string]any{"command": "rm a.txt"}},
				{ID: "call_b", Name: "bash", Input: map[string]any{"command": "touch b.txt"}},
			},
		},
		// Only call_a got a result before the prior process died.
		{Role: llm.RoleTool, ToolUseID: "call_a", ToolResult: "ok"},
		{Role: llm.RoleUser, Content: "hi"},
	}
	out := repairToolCallPairs(in)
	if len(out) != len(in)+1 {
		t.Fatalf("expected one synthetic tool result inserted, got %d -> %d", len(in), len(out))
	}
	// Synthetic result for call_b must be appended right after the assistant
	// turn, before any existing tool results, so OpenAI's strict ordering
	// (assistant tool_calls immediately followed by tool messages) holds.
	if out[2].Role != llm.RoleTool || out[2].ToolUseID != "call_b" || !out[2].ToolError {
		t.Fatalf("synthetic result placed wrong at index 2: %+v", out[2])
	}
	if out[3].Role != llm.RoleTool || out[3].ToolUseID != "call_a" {
		t.Fatalf("existing call_a result got displaced: %+v", out[3])
	}
	if out[4].Role != llm.RoleUser {
		t.Fatalf("subsequent user message got displaced: %+v", out[4])
	}
}

// TestLoop_AtomicityOnToolError covers the write-side defense: every
// tool_call ID in the assistant turn must end up with a tool result message
// in memory, even when ctx is canceled mid-call.
func TestLoop_AtomicityOnCanceledRun(t *testing.T) {
	provider := &stubProvider{
		responses: []*llm.Response{{
			StopReason: llm.StopToolUse,
			Message: llm.Message{
				Role: llm.RoleAssistant,
				ToolCalls: []llm.ToolCall{
					{ID: "t1", Name: "echo", Input: map[string]any{"msg": "go"}},
					{ID: "t2", Name: "echo", Input: map[string]any{"msg": "go"}},
				},
			},
		}},
	}
	reg := tools.NewRegistry()
	reg.Register(&fakeTool{})

	mem := memory.NewInMemory()
	a := New(provider, reg, mem, Options{Model: "m", MaxIterations: 5})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-canceled: the loop should still flush synthetic results.

	_, _ = a.Run(ctx, "s", "make files")

	hist, _ := mem.LoadMessages(context.Background(), "s")
	have := map[string]bool{}
	for _, m := range hist {
		if m.Role == "tool" {
			have[m.ToolUseID] = true
		}
	}
	if !have["t1"] || !have["t2"] {
		t.Fatalf("expected tool results for both call IDs, got %v in %d msgs", have, len(hist))
	}
}

// stubRouter is a Router that returns a canned skill name (or an error).
type stubRouter struct {
	pick string
	err  error
	hits int
}

func (s *stubRouter) Route(_ context.Context, _ string, _ []*skills.Skill) (string, error) {
	s.hits++
	return s.pick, s.err
}

// TestLoop_RouterWrapsAndPersists covers the cache-stable injection path:
// the matched skill's prompt MUST end up inside the persisted user message
// (so prefix cache stays warm on replay), the agent's SystemPrompt MUST be
// untouched, and EventRouter MUST fire so the CLI can show the decision.
func TestLoop_RouterWrapsAndPersists(t *testing.T) {
	provider := &stubProvider{responses: []*llm.Response{
		{StopReason: llm.StopEnd, Message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}},
	}}
	mem := memory.NewInMemory()
	a := New(provider, tools.NewRegistry(), mem, Options{Model: "m", SystemPrompt: "FIXED-SYSTEM"})

	reg := skills.NewRegistry()
	// Stuff a skill into the registry directly via Save+TempDir to avoid
	// reaching into unexported fields. We just need *one* skill the router
	// can return.
	dir := t.TempDir()
	if _, err := reg.Save(&skills.Skill{
		Name:        "nginx-triage",
		Description: "triage nginx",
		Prompt:      "Look at access.log first.",
	}, dir); err != nil {
		t.Fatalf("save skill: %v", err)
	}
	a.SetSkills(reg, dir, nil)
	a.SetRouter(&stubRouter{pick: "nginx-triage"})

	gotEvents := []Event{}
	a.SetListener(func(e Event) { gotEvents = append(gotEvents, e) })

	if _, err := a.Run(context.Background(), "s", "nginx 502"); err != nil {
		t.Fatalf("run: %v", err)
	}

	// 1) Persisted user message contains the wrap, not the bare input.
	hist, _ := mem.LoadMessages(context.Background(), "s")
	var userMsg memory.Message
	for _, m := range hist {
		if m.Role == "user" {
			userMsg = m
			break
		}
	}
	if !strings.Contains(userMsg.Content, "<skill name=\"nginx-triage\">") {
		t.Errorf("persisted user message missing skill wrap: %q", userMsg.Content)
	}
	if !strings.Contains(userMsg.Content, "Look at access.log first.") {
		t.Errorf("persisted user message missing skill prompt body: %q", userMsg.Content)
	}
	if !strings.Contains(userMsg.Content, "nginx 502") {
		t.Errorf("user input was lost: %q", userMsg.Content)
	}

	// 2) EventRouter emitted with the skill name.
	sawRouter := false
	for _, e := range gotEvents {
		if e.Kind == EventRouter && e.Text == "nginx-triage" {
			sawRouter = true
		}
	}
	if !sawRouter {
		t.Errorf("expected EventRouter for nginx-triage, events=%+v", gotEvents)
	}

	// 3) SystemPrompt MUST be untouched — that's the whole reason we wrap
	// the user message instead of mutating req.System.
	if a.opts.SystemPrompt != "FIXED-SYSTEM" {
		t.Errorf("SystemPrompt was mutated by routing: %q", a.opts.SystemPrompt)
	}
}

// TestLoop_RouterErrorFallsBackToBareInput: a router failure must not block
// the user's turn — log it via EventError and persist the bare input.
func TestLoop_RouterErrorFallsBackToBareInput(t *testing.T) {
	provider := &stubProvider{responses: []*llm.Response{
		{StopReason: llm.StopEnd, Message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}},
	}}
	mem := memory.NewInMemory()
	a := New(provider, tools.NewRegistry(), mem, Options{Model: "m"})

	reg := skills.NewRegistry()
	dir := t.TempDir()
	if _, err := reg.Save(&skills.Skill{Name: "x", Description: "x", Prompt: "x"}, dir); err != nil {
		t.Fatalf("save skill: %v", err)
	}
	a.SetSkills(reg, dir, nil)
	a.SetRouter(&stubRouter{err: errors.New("boom")})

	if _, err := a.Run(context.Background(), "s", "the bare input"); err != nil {
		t.Fatalf("run: %v", err)
	}
	hist, _ := mem.LoadMessages(context.Background(), "s")
	for _, m := range hist {
		if m.Role == "user" {
			if m.Content != "the bare input" {
				t.Errorf("router error should leave user msg bare, got %q", m.Content)
			}
			return
		}
	}
	t.Errorf("no user message persisted")
}

func TestLoop_RespectsMaxIterations(t *testing.T) {
	loop := func() *llm.Response {
		return &llm.Response{
			StopReason: llm.StopToolUse,
			Message: llm.Message{
				Role: llm.RoleAssistant,
				ToolCalls: []llm.ToolCall{{
					ID: "x", Name: "echo", Input: map[string]any{"msg": "again"},
				}},
			},
		}
	}
	provider := &stubProvider{responses: []*llm.Response{loop(), loop(), loop(), loop()}}
	reg := tools.NewRegistry()
	reg.Register(&fakeTool{})
	a := New(provider, reg, memory.NewInMemory(), Options{Model: "m", MaxIterations: 2})

	_, err := a.Run(context.Background(), "s2", "loop forever")
	if err == nil {
		t.Fatal("expected max iterations error")
	}
}
