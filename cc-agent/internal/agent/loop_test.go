package agent

import (
	"context"
	"errors"
	"testing"

	"cc-agent/internal/llm"
	"cc-agent/internal/memory"
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
