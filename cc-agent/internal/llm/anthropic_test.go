package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnthropicProvider_ToolUseRoundtrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("missing api key header")
		}
		var req anthropicReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode req: %v", err)
		}
		if req.Model != "claude-test" {
			t.Errorf("model = %q", req.Model)
		}
		if len(req.Tools) != 1 || req.Tools[0].Name != "bash" {
			t.Errorf("tools not forwarded: %+v", req.Tools)
		}
		_ = json.NewEncoder(w).Encode(anthropicResp{
			ID: "msg_1", Type: "message", Role: "assistant", Model: "claude-test",
			Content: []anthropicContent{
				{Type: "text", Text: "I will inspect the system."},
				{Type: "tool_use", ID: "toolu_1", Name: "bash", Input: map[string]any{"command": "uptime"}},
			},
			StopReason: "tool_use",
		})
	}))
	defer srv.Close()

	p, err := NewAnthropic(AnthropicOptions{APIKey: "test-key", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	resp, err := p.Complete(context.Background(), Request{
		Model:    "claude-test",
		System:   "you are a test bot",
		Messages: []Message{{Role: RoleUser, Content: "check uptime"}},
		Tools: []ToolDef{{
			Name: "bash", Description: "shell",
			InputSchema: map[string]any{"type": "object"},
		}},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if resp.StopReason != StopToolUse {
		t.Errorf("stop reason = %v", resp.StopReason)
	}
	if !strings.Contains(resp.Message.Content, "inspect") {
		t.Errorf("text not captured: %q", resp.Message.Content)
	}
	if len(resp.Message.ToolCalls) != 1 || resp.Message.ToolCalls[0].Name != "bash" {
		t.Errorf("tool call missing: %+v", resp.Message.ToolCalls)
	}
}

func TestAnthropicProvider_PropagatesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"type":"invalid_request","message":"bad model"}}`))
	}))
	defer srv.Close()

	p, _ := NewAnthropic(AnthropicOptions{APIKey: "k", BaseURL: srv.URL})
	_, err := p.Complete(context.Background(), Request{Model: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("err = %v", err)
	}
}
