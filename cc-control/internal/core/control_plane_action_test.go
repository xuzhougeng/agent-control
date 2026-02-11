package core

import (
	"encoding/base64"
	"path/filepath"
	"testing"
)

type fakeAgentConn struct {
	msgs []Envelope
}

func (f *fakeAgentConn) Send(msg Envelope) error {
	f.msgs = append(f.msgs, msg)
	return nil
}

func setupActionTestControlPlane(t *testing.T, prompt string) (*ControlPlane, *fakeAgentConn, string, string) {
	t.Helper()
	auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
	cp, err := NewControlPlane(Config{AuditPath: auditPath})
	if err != nil {
		t.Fatalf("new control plane: %v", err)
	}
	t.Cleanup(func() { _ = cp.Close() })

	conn := &fakeAgentConn{}
	sessionID := "s1"
	eventID := "e1"
	cp.mu.Lock()
	cp.servers["srv"] = &Server{ServerID: "srv", Status: ServerOnline}
	cp.agentConns["srv"] = conn
	cp.sessions[sessionID] = &Session{
		SessionID:        sessionID,
		ServerID:         "srv",
		Status:           SessionRunning,
		AwaitingApproval: true,
		PendingEventID:   eventID,
	}
	cp.sessionEvents[sessionID] = []SessionEvent{
		{
			EventID:    eventID,
			SessionID:  sessionID,
			ServerID:   "srv",
			Kind:       "approval_needed",
			PromptText: prompt,
			TsMS:       1,
		},
	}
	cp.mu.Unlock()
	return cp, conn, sessionID, eventID
}

func lastPTYInput(t *testing.T, conn *fakeAgentConn) string {
	t.Helper()
	if len(conn.msgs) == 0 {
		t.Fatal("expected at least one message to agent")
	}
	msg := conn.msgs[len(conn.msgs)-1]
	if msg.Type != "pty_in" {
		t.Fatalf("expected pty_in, got %q", msg.Type)
	}
	raw, err := base64.StdEncoding.DecodeString(msg.DataB64)
	if err != nil {
		t.Fatalf("decode pty input: %v", err)
	}
	return string(raw)
}

func TestHandleClientAction_ApproveMenuUsesEnter(t *testing.T) {
	prompt := "Do you want to create abc?\n1. Yes\n2. Yes, allow all edits during this session (shift+tab)\n3. No\nEsc to cancel · Tab to amend"
	cp, conn, sessionID, eventID := setupActionTestControlPlane(t, prompt)

	if err := cp.HandleClientAction("ui:test", sessionID, ActionRequest{Kind: "approve", EventID: eventID}); err != nil {
		t.Fatalf("approve action failed: %v", err)
	}
	if got := lastPTYInput(t, conn); got != "\r" {
		t.Fatalf("menu approve should send Enter, got %q", got)
	}
}

func TestHandleClientAction_RejectMenuUsesEscape(t *testing.T) {
	prompt := "Do you want to create abc?\n1. Yes\n2. Yes, allow all edits during this session (shift+tab)\n3. No\nEsc to cancel · Tab to amend"
	cp, conn, sessionID, eventID := setupActionTestControlPlane(t, prompt)

	if err := cp.HandleClientAction("ui:test", sessionID, ActionRequest{Kind: "reject", EventID: eventID}); err != nil {
		t.Fatalf("reject action failed: %v", err)
	}
	if got := lastPTYInput(t, conn); got != "\u001b" {
		t.Fatalf("menu reject should send Escape, got %q", got)
	}
}

func TestHandleClientAction_PlainPromptUsesYN(t *testing.T) {
	prompt := "Do you want to continue? [y/N]"
	cp1, conn1, sessionID1, eventID1 := setupActionTestControlPlane(t, prompt)
	if err := cp1.HandleClientAction("ui:test", sessionID1, ActionRequest{Kind: "approve", EventID: eventID1}); err != nil {
		t.Fatalf("approve action failed: %v", err)
	}
	if got := lastPTYInput(t, conn1); got != "y\n" {
		t.Fatalf("plain approve should send y\\n, got %q", got)
	}

	cp2, conn2, sessionID2, eventID2 := setupActionTestControlPlane(t, prompt)
	if err := cp2.HandleClientAction("ui:test", sessionID2, ActionRequest{Kind: "reject", EventID: eventID2}); err != nil {
		t.Fatalf("reject action failed: %v", err)
	}
	if got := lastPTYInput(t, conn2); got != "n\n" {
		t.Fatalf("plain reject should send n\\n, got %q", got)
	}
}

func TestHandleClientAction_StaleEventIDStillExecutesCurrentPending(t *testing.T) {
	prompt := "Do you want to create abc?\n1. Yes\n2. Yes, allow all edits during this session (shift+tab)\n3. No\nEsc to cancel · Tab to amend"
	cp, conn, sessionID, _ := setupActionTestControlPlane(t, prompt)

	if err := cp.HandleClientAction("ui:test", sessionID, ActionRequest{Kind: "approve", EventID: "stale-event-id"}); err != nil {
		t.Fatalf("approve with stale event_id should still succeed, got: %v", err)
	}
	if got := lastPTYInput(t, conn); got != "\r" {
		t.Fatalf("menu approve should send Enter for current pending event, got %q", got)
	}
}
