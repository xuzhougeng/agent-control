package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"cc-agent/internal/agent"
	"cc-agent/internal/llm"
	"cc-agent/internal/memory"
	"cc-agent/internal/tools"
)

func TestNormalizeWSURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ws://x:1/y", "ws://x:1/y"},
		{"wss://x/y", "wss://x/y"},
		{"http://x:1/y", "ws://x:1/y"},
		{"https://x:1/y", "wss://x:1/y"},
	}
	for _, c := range cases {
		got, err := normalizeWSURL(c.in)
		if err != nil {
			t.Errorf("%s: err %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: got %s want %s", c.in, got, c.want)
		}
	}
	if _, err := normalizeWSURL(""); err == nil {
		t.Error("empty url should error")
	}
}

func TestValidateCwd(t *testing.T) {
	if err := validateCwd("/tmp/x", nil); err != nil {
		t.Errorf("no roots = unrestricted: %v", err)
	}
	if err := validateCwd("/var/log", []string{"/tmp", "/var"}); err != nil {
		t.Errorf("under root /var: %v", err)
	}
	if err := validateCwd("/etc/passwd", []string{"/tmp", "/var"}); err == nil {
		t.Error("expected reject for /etc")
	}
	if err := validateCwd("/var", []string{"/var"}); err != nil {
		t.Errorf("exact match: %v", err)
	}
	if err := validateCwd("", []string{"/tmp"}); err == nil {
		t.Error("empty cwd should error")
	}
}

// stubProvider returns a single canned response per Complete call.
type stubProvider struct{ next *llm.Response }

func (s *stubProvider) Name() string { return "stub" }
func (s *stubProvider) Complete(_ context.Context, _ llm.Request) (*llm.Response, error) {
	r := s.next
	s.next = &llm.Response{StopReason: llm.StopEnd, Message: llm.Message{Role: llm.RoleAssistant, Content: "all done."}}
	return r, nil
}

// recordingTool is a Tool that records calls and returns a fixed string.
type recordingTool struct {
	name   string
	result string
	calls  int
}

func (r *recordingTool) Name() string                                                  { return r.name }
func (r *recordingTool) Description() string                                           { return "echo" }
func (r *recordingTool) InputSchema() map[string]any                                   { return map[string]any{"type": "object"} }
func (r *recordingTool) Run(_ context.Context, _ map[string]any) (string, error) {
	r.calls++
	return r.result, nil
}

// TestStreamingEmitsProgressBeforeFinal drives an agent loop that issues one
// tool call. The control client should send a progress chat_out for the
// tool_use, a progress chat_out for the tool_result, and finally a meta.final
// chat_out — in that order.
func TestStreamingEmitsProgressBeforeFinal(t *testing.T) {
	upgrader := websocket.Upgrader{}
	chatOutCh := make(chan Envelope, 16)
	registered := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade: %v", err)
		}
		defer conn.Close()
		var register Envelope
		_ = conn.ReadJSON(&register)
		ack := newEnvelope("register_ok", register.ServerID, "")
		_ = conn.WriteJSON(ack)
		close(registered)

		startData, _ := json.Marshal(StartChatPayload{Cwd: "/tmp"})
		_ = conn.WriteJSON(Envelope{Type: "start_chat", ServerID: register.ServerID, SessionID: "s-1", InstanceID: "i-1", Data: startData})
		chatData, _ := json.Marshal(ChatInPayload{MessageID: "m-base", Content: "use the tool"})
		_ = conn.WriteJSON(Envelope{Type: "chat_in", ServerID: register.ServerID, SessionID: "s-1", InstanceID: "i-1", Data: chatData})

		for {
			var msg Envelope
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			if msg.Type == "chat_out" {
				select {
				case chatOutCh <- msg:
				default:
				}
				var p ChatOutPayload
				_ = json.Unmarshal(msg.Data, &p)
				var meta ChatOutMeta
				_ = json.Unmarshal(p.Meta, &meta)
				if meta.Final {
					return
				}
			}
		}
	}))
	defer srv.Close()

	wsURL, _ := url.Parse(srv.URL)
	wsURL.Scheme = "ws"
	wsURL.Path = "/ws/agent"

	provider := &stubProvider{next: &llm.Response{
		StopReason: llm.StopToolUse,
		Message: llm.Message{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID: "tc-1", Name: "echo", Input: map[string]any{"msg": "hi"},
			}},
		},
	}}
	// stubProvider's second call (after tool result is appended) returns the
	// canned end-turn response with content "all done.".
	mem := memory.NewInMemory()
	reg := tools.NewRegistry()
	reg.Register(&recordingTool{name: "echo", result: "ECHOED-OUTPUT"})
	ag := agent.New(provider, reg, mem, agent.Options{Model: "stub"})

	c, err := New(Options{
		URL: wsURL.String(), Token: "test-token", ServerID: "srv-test",
		Agent: ag, HeartbeatEvery: time.Hour,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() { _ = c.Run(ctx) }()

	select {
	case <-registered:
	case <-ctx.Done():
		t.Fatalf("register timeout")
	}

	collected := []ChatOutMeta{}
	contents := []string{}
	deadline := time.After(4 * time.Second)
loop:
	for {
		select {
		case env := <-chatOutCh:
			var p ChatOutPayload
			_ = json.Unmarshal(env.Data, &p)
			var meta ChatOutMeta
			_ = json.Unmarshal(p.Meta, &meta)
			collected = append(collected, meta)
			contents = append(contents, p.Content)
			if meta.Final {
				break loop
			}
		case <-deadline:
			break loop
		}
	}
	if len(collected) < 3 {
		t.Fatalf("expected at least 3 chat_out (tool_use progress, tool_result progress, final), got %d: %+v", len(collected), contents)
	}
	if !collected[0].Progress || collected[0].Final {
		t.Errorf("first message must be progress, got %+v", collected[0])
	}
	last := collected[len(collected)-1]
	if !last.Final || last.Progress {
		t.Errorf("last message must be final, got %+v", last)
	}
	if len(last.Operations) < 2 {
		t.Errorf("final must carry at least 2 operations summary, got %v", last.Operations)
	}
	if !strings.Contains(contents[0], "echo") {
		t.Errorf("first progress should mention tool name, got %q", contents[0])
	}
}

// TestEndToEnd_StartChatAndChatIn drives a full register → start_chat →
// chat_in → chat_out roundtrip against an httptest WS server pretending to be
// cc-control.
func TestEndToEnd_StartChatAndChatIn(t *testing.T) {
	upgrader := websocket.Upgrader{}
	type recorded struct {
		register Envelope
		chatOut  Envelope
	}
	rec := &recorded{}
	registerOK := make(chan struct{})
	chatOutCh := make(chan Envelope, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing auth header")
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade: %v", err)
		}
		defer conn.Close()
		if err := conn.ReadJSON(&rec.register); err != nil {
			t.Fatalf("read register: %v", err)
		}
		if rec.register.Type != "register" {
			t.Fatalf("first frame must be register, got %s", rec.register.Type)
		}
		ack := newEnvelope("register_ok", rec.register.ServerID, "")
		ack.Data, _ = json.Marshal(map[string]any{"protocol_version": ProtocolVersion})
		_ = conn.WriteJSON(ack)
		close(registerOK)

		// Send a start_chat then chat_in.
		startData, _ := json.Marshal(StartChatPayload{Cwd: "/tmp"})
		startMsg := Envelope{Type: "start_chat", ServerID: rec.register.ServerID, SessionID: "s-1", InstanceID: "i-1", Data: startData}
		_ = conn.WriteJSON(startMsg)

		chatData, _ := json.Marshal(ChatInPayload{MessageID: "m-1", Content: "ping"})
		chatIn := Envelope{Type: "chat_in", ServerID: rec.register.ServerID, SessionID: "s-1", InstanceID: "i-1", Data: chatData}
		_ = conn.WriteJSON(chatIn)

		// Read until we see chat_out or heartbeat etc.
		for {
			var msg Envelope
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			if msg.Type == "chat_out" {
				rec.chatOut = msg
				select {
				case chatOutCh <- msg:
				default:
				}
				return
			}
		}
	}))
	defer srv.Close()

	wsURL, _ := url.Parse(srv.URL)
	wsURL.Scheme = "ws"
	wsURL.Path = "/ws/agent"

	provider := &stubProvider{next: &llm.Response{
		StopReason: llm.StopEnd,
		Message:    llm.Message{Role: llm.RoleAssistant, Content: "pong-from-stub"},
	}}
	mem := memory.NewInMemory()
	reg := tools.NewRegistry()
	ag := agent.New(provider, reg, mem, agent.Options{Model: "stub"})

	c, err := New(Options{
		URL: wsURL.String(), Token: "test-token", ServerID: "srv-test",
		Agent: ag, AllowedRoots: nil, HeartbeatEvery: time.Hour,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() { _ = c.Run(ctx) }()

	select {
	case <-registerOK:
	case <-ctx.Done():
		t.Fatalf("register timeout")
	}
	var regPayload RegisterPayload
	_ = json.Unmarshal(rec.register.Data, &regPayload)
	if regPayload.ProtocolVersion != ProtocolVersion {
		t.Errorf("register protocol version = %d, want %d", regPayload.ProtocolVersion, ProtocolVersion)
	}
	if regPayload.ServerID != "srv-test" {
		t.Errorf("register server_id = %q", regPayload.ServerID)
	}

	select {
	case out := <-chatOutCh:
		var p ChatOutPayload
		if err := json.Unmarshal(out.Data, &p); err != nil {
			t.Fatalf("decode chat_out: %v", err)
		}
		if !strings.HasPrefix(p.MessageID, "m-1/") {
			t.Errorf("chat_out message_id = %q, want prefix m-1/", p.MessageID)
		}
		if !strings.Contains(p.Content, "pong-from-stub") {
			t.Errorf("chat_out content = %q, want contains pong-from-stub", p.Content)
		}
		if out.SessionID != "s-1" {
			t.Errorf("chat_out session_id = %q", out.SessionID)
		}
		var meta ChatOutMeta
		if err := json.Unmarshal(p.Meta, &meta); err != nil {
			t.Fatalf("decode meta: %v", err)
		}
		if !meta.Final {
			t.Errorf("expected meta.final=true on closing message, got %+v", meta)
		}
	case <-ctx.Done():
		t.Fatalf("chat_out timeout")
	}
}
