package core

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func setupChatTestCP(t *testing.T) (*ControlPlane, *fakeAgentConn, string) {
	t.Helper()
	auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
	cp, err := NewControlPlane(Config{AuditPath: auditPath})
	if err != nil {
		t.Fatalf("new control plane: %v", err)
	}
	t.Cleanup(func() { _ = cp.Close() })

	conn := &fakeAgentConn{}
	tenantID := "t1"
	cp.mu.Lock()
	cp.servers["srv"] = &Server{TenantID: tenantID, ServerID: "srv", Status: ServerOnline}
	cp.agentConns["srv"] = conn
	cp.mu.Unlock()

	sess, err := cp.CreateSession("ui:test", tenantID, StartSessionRequest{
		ServerID:    "srv",
		SessionType: SessionTypeChat,
		Cwd:         "/tmp",
	})
	if err != nil {
		t.Fatalf("create chat session: %v", err)
	}
	return cp, conn, sess.SessionID
}

func TestCreateSession_ChatType(t *testing.T) {
	cp, conn, sessionID := setupChatTestCP(t)

	cp.mu.RLock()
	sess := cp.sessions[sessionID]
	cp.mu.RUnlock()
	if sess.SessionType != SessionTypeChat {
		t.Fatalf("expected chat, got %s", sess.SessionType)
	}

	if len(conn.msgs) == 0 {
		t.Fatal("expected start_chat message to agent")
	}
	if conn.msgs[0].Type != "start_chat" {
		t.Fatalf("expected start_chat, got %s", conn.msgs[0].Type)
	}
}

func TestHandleClientChatIn(t *testing.T) {
	cp, conn, sessionID := setupChatTestCP(t)

	err := cp.HandleClientChatIn("ui:test", "t1", sessionID, "hello", nil)
	if err != nil {
		t.Fatalf("chat_in failed: %v", err)
	}

	found := false
	for _, m := range conn.msgs {
		if m.Type == "chat_in" {
			found = true
			var payload struct {
				MessageID    string            `json:"message_id"`
				Content      string            `json:"content"`
				ContentParts []ChatContentPart `json:"content_parts"`
			}
			if err := json.Unmarshal(m.Data, &payload); err != nil {
				t.Fatalf("unmarshal chat_in: %v", err)
			}
			if payload.Content != "hello" {
				t.Fatalf("expected content 'hello', got %q", payload.Content)
			}
			if len(payload.ContentParts) != 1 || payload.ContentParts[0].Type != "text" || payload.ContentParts[0].Text != "hello" {
				t.Fatalf("unexpected content parts: %+v", payload.ContentParts)
			}
		}
	}
	if !found {
		t.Fatal("no chat_in message sent to agent")
	}

	msgs, err := cp.GetChatHistory("t1", sessionID)
	if err != nil {
		t.Fatalf("get history: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Role != "user" || msgs[0].Content != "hello" {
		t.Fatalf("unexpected history: %+v", msgs)
	}
}

func TestHandleClientChatIn_WithImageParts(t *testing.T) {
	cp, conn, sessionID := setupChatTestCP(t)
	imgData := "AQIDBA=="
	parts := []ChatContentPart{
		{Type: "text", Text: "describe this"},
		{
			Type: "image",
			Source: &ChatImageSource{
				Type:      "base64",
				MediaType: "image/png",
				Data:      imgData,
			},
		},
	}

	err := cp.HandleClientChatIn("ui:test", "t1", sessionID, "", parts)
	if err != nil {
		t.Fatalf("chat_in with image failed: %v", err)
	}

	var payload struct {
		Content      string            `json:"content"`
		ContentParts []ChatContentPart `json:"content_parts"`
	}
	found := false
	for _, m := range conn.msgs {
		if m.Type != "chat_in" {
			continue
		}
		found = true
		if err := json.Unmarshal(m.Data, &payload); err != nil {
			t.Fatalf("unmarshal chat_in payload: %v", err)
		}
		break
	}
	if !found {
		t.Fatal("no chat_in message sent to agent")
	}
	if payload.Content != "describe this\n\n[image:image/png]" {
		t.Fatalf("unexpected plain content: %q", payload.Content)
	}
	if len(payload.ContentParts) != 2 {
		t.Fatalf("unexpected content parts count: %+v", payload.ContentParts)
	}
	if payload.ContentParts[1].Source == nil || payload.ContentParts[1].Source.Data != imgData {
		t.Fatalf("unexpected image payload: %+v", payload.ContentParts[1].Source)
	}

	msgs, err := cp.GetChatHistory("t1", sessionID)
	if err != nil {
		t.Fatalf("get history: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("unexpected history len: %d", len(msgs))
	}
	var meta struct {
		ContentParts []ChatContentPart `json:"content_parts"`
	}
	if err := json.Unmarshal(msgs[0].Meta, &meta); err != nil {
		t.Fatalf("unmarshal user meta: %v", err)
	}
	if len(meta.ContentParts) != 2 {
		t.Fatalf("unexpected history meta parts: %+v", meta.ContentParts)
	}
}

func TestHandleChatOut(t *testing.T) {
	cp, _, sessionID := setupChatTestCP(t)

	payload, _ := json.Marshal(map[string]any{
		"message_id": "m1",
		"content":    "echo hello",
		"meta": map[string]any{
			"operations": []string{"tool_use: Bash", "result: status=ok"},
		},
	})
	cp.HandleChatOut("srv", sessionID, payload)

	msgs, err := cp.GetChatHistory("t1", sessionID)
	if err != nil {
		t.Fatalf("get history: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Role != "assistant" || msgs[0].Content != "echo hello" {
		t.Fatalf("unexpected history: %+v", msgs)
	}
	var meta struct {
		Operations []string `json:"operations"`
	}
	if err := json.Unmarshal(msgs[0].Meta, &meta); err != nil {
		t.Fatalf("unmarshal meta: %v", err)
	}
	if len(meta.Operations) != 2 {
		t.Fatalf("unexpected meta operations: %+v", meta.Operations)
	}
}

func TestHandleChatExit(t *testing.T) {
	cp, _, sessionID := setupChatTestCP(t)

	code := 0
	payload, _ := json.Marshal(PTYExit{ExitCode: &code, Reason: "done"})
	cp.HandleChatExit("srv", sessionID, payload)

	cp.mu.RLock()
	sess := cp.sessions[sessionID]
	cp.mu.RUnlock()
	if sess.Status != SessionExited {
		t.Fatalf("expected exited, got %s", sess.Status)
	}
}

func TestChatIn_RejectsNonChatSession(t *testing.T) {
	auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
	cp, err := NewControlPlane(Config{AuditPath: auditPath})
	if err != nil {
		t.Fatalf("new control plane: %v", err)
	}
	t.Cleanup(func() { _ = cp.Close() })

	conn := &fakeAgentConn{}
	cp.mu.Lock()
	cp.servers["srv"] = &Server{TenantID: "t1", ServerID: "srv", Status: ServerOnline}
	cp.agentConns["srv"] = conn
	cp.mu.Unlock()

	sess, err := cp.CreateSession("ui:test", "t1", StartSessionRequest{
		ServerID: "srv",
		Cwd:      "/tmp",
		Cols:     80,
		Rows:     24,
	})
	if err != nil {
		t.Fatalf("create pty session: %v", err)
	}

	err = cp.HandleClientChatIn("ui:test", "t1", sess.SessionID, "hello", nil)
	if err == nil {
		t.Fatal("expected error for chat_in on pty session")
	}
}

func TestChatHistoryLimit(t *testing.T) {
	cp, _, sessionID := setupChatTestCP(t)

	for i := 0; i < MaxChatHistory+50; i++ {
		cp.mu.Lock()
		cp.appendChatMessage(sessionID, ChatMessage{
			MessageID: "m",
			Role:      "user",
			Content:   "x",
		})
		cp.mu.Unlock()
	}

	msgs, err := cp.GetChatHistory("t1", sessionID)
	if err != nil {
		t.Fatalf("get history: %v", err)
	}
	if len(msgs) != MaxChatHistory {
		t.Fatalf("expected %d messages, got %d", MaxChatHistory, len(msgs))
	}
}

func TestGetChatHistory_TenantIsolation(t *testing.T) {
	cp, _, sessionID := setupChatTestCP(t)

	_, err := cp.GetChatHistory("other-tenant", sessionID)
	if err == nil {
		t.Fatal("expected error for wrong tenant")
	}
}
