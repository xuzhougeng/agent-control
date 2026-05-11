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

func TestRewind_DropsLastUserTurn(t *testing.T) {
	provider := &stubProvider{
		responses: []*llm.Response{
			{StopReason: llm.StopEnd, Message: llm.Message{Role: llm.RoleAssistant, Content: "first reply"}},
			{StopReason: llm.StopEnd, Message: llm.Message{Role: llm.RoleAssistant, Content: "second reply"}},
		},
	}
	mem := memory.NewInMemory()
	a := New(provider, tools.NewRegistry(), mem, Options{Model: "m"})

	ctx := context.Background()
	if _, err := a.Run(ctx, "s", "first question"); err != nil {
		t.Fatalf("run 1: %v", err)
	}
	if _, err := a.Run(ctx, "s", "second question"); err != nil {
		t.Fatalf("run 2: %v", err)
	}
	dropped, err := a.Rewind(ctx, "s", 1)
	if err != nil {
		t.Fatalf("rewind: %v", err)
	}
	// Dropped: the 2nd user message + the 2nd assistant reply = 2 messages.
	if dropped != 2 {
		t.Errorf("expected 2 dropped, got %d", dropped)
	}
	hist, _ := mem.LoadMessages(ctx, "s")
	if len(hist) != 2 {
		t.Fatalf("expected 2 messages remaining, got %d: %+v", len(hist), hist)
	}
	if hist[0].Role != "user" || hist[0].Content != "first question" {
		t.Errorf("first message wrong: %+v", hist[0])
	}
	if hist[1].Role != "assistant" || hist[1].Content != "first reply" {
		t.Errorf("second message wrong: %+v", hist[1])
	}
}

func TestRewind_RejectsTooMany(t *testing.T) {
	provider := &stubProvider{responses: []*llm.Response{
		{StopReason: llm.StopEnd, Message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}},
	}}
	mem := memory.NewInMemory()
	a := New(provider, tools.NewRegistry(), mem, Options{Model: "m"})
	if _, err := a.Run(context.Background(), "s", "only question"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := a.Rewind(context.Background(), "s", 5); err == nil {
		t.Error("rewind 5 from 1-turn session should error")
	}
	hist, _ := mem.LoadMessages(context.Background(), "s")
	if len(hist) != 2 {
		t.Errorf("memory should be untouched on error, got %d msgs", len(hist))
	}
}

func TestRewind_DropsAllOnFullRewind(t *testing.T) {
	provider := &stubProvider{responses: []*llm.Response{
		{StopReason: llm.StopEnd, Message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}},
	}}
	mem := memory.NewInMemory()
	a := New(provider, tools.NewRegistry(), mem, Options{Model: "m"})
	if _, err := a.Run(context.Background(), "s", "only question"); err != nil {
		t.Fatalf("run: %v", err)
	}
	dropped, err := a.Rewind(context.Background(), "s", 1)
	if err != nil {
		t.Fatalf("rewind: %v", err)
	}
	if dropped != 2 {
		t.Errorf("expected 2 dropped, got %d", dropped)
	}
	hist, _ := mem.LoadMessages(context.Background(), "s")
	if len(hist) != 0 {
		t.Errorf("expected empty session, got %d msgs", len(hist))
	}
}

// fakeProvider lets a closure-based Complete satisfy llm.Provider; useful
// when a test needs to inspect the incoming request.
type fakeProvider struct {
	complete func(ctx context.Context, req llm.Request) (*llm.Response, error)
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Complete(ctx context.Context, req llm.Request) (*llm.Response, error) {
	return f.complete(ctx, req)
}

func TestAgent_RunWithForcedSkill_BypassesRouter(t *testing.T) {
	ctx := context.Background()
	mem := memory.NewInMemory()

	// Provider records the request and returns a trivial reply.
	var lastUser string
	prov := &fakeProvider{
		complete: func(ctx context.Context, req llm.Request) (*llm.Response, error) {
			for _, m := range req.Messages {
				if m.Role == llm.RoleUser {
					lastUser = m.Content
				}
			}
			return &llm.Response{
				Message:    llm.Message{Role: llm.RoleAssistant, Content: "ok"},
				StopReason: llm.StopEnd,
			}, nil
		},
	}
	reg := skills.NewRegistry()
	pinned := &skills.Skill{
		Name: "kernel-probe", Description: "probe kernel",
		Prompt: "PROBE-PROMPT",
	}
	skillsDir := t.TempDir()
	if _, err := reg.Save(pinned, skillsDir); err != nil {
		t.Fatalf("save skill: %v", err)
	}
	// Router that would pick a DIFFERENT skill if invoked — we assert it is
	// NOT invoked.
	routed := false
	router := skills.RouterFunc(func(_ context.Context, _ string, _ []*skills.Skill) (string, error) {
		routed = true
		return "wrong-skill", nil
	})

	ag := New(prov, tools.NewRegistry(), mem, Options{Model: "x", MaxTokens: 8})
	ag.SetSkills(reg, skillsDir, nil)
	ag.SetRouter(router)

	var events []Event
	listener := func(e Event) { events = append(events, e) }

	if _, err := ag.RunWithForcedSkill(ctx, "s1", "hello", pinned, listener); err != nil {
		t.Fatalf("RunWithForcedSkill: %v", err)
	}
	if routed {
		t.Error("router should NOT have been invoked under RunWithForcedSkill")
	}
	if !strings.Contains(lastUser, "PROBE-PROMPT") {
		t.Errorf("expected pinned skill prompt to be folded into user msg; got %q", lastUser)
	}
	if !strings.Contains(lastUser, "kernel-probe") {
		t.Errorf("expected pinned skill name in wrap; got %q", lastUser)
	}
	// Confirm we emitted the (pinned) router event.
	foundEvent := false
	for _, e := range events {
		if e.Kind == EventRouter && strings.Contains(e.Text, "kernel-probe") && strings.Contains(e.Text, "pinned") {
			foundEvent = true
		}
	}
	if !foundEvent {
		t.Errorf("expected EventRouter with pinned marker; got events=%+v", events)
	}

	// Direct memory assertion: the persisted user message should be the
	// wrapped form, not the raw "hello" — proving runTurnBody saved the
	// wrap-output, not the pre-wrap input.
	hist, err := mem.LoadMessages(ctx, "s1")
	if err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}
	var persisted string
	for _, m := range hist {
		if m.Role == "user" {
			persisted = m.Content
			break
		}
	}
	if !strings.Contains(persisted, "PROBE-PROMPT") {
		t.Errorf("persisted user message missing skill wrap: %q", persisted)
	}
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
