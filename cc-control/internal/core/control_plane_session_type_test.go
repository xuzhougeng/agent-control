package core

import (
	"path/filepath"
	"testing"
)

func setupSessionTypeTestCP(t *testing.T, serverOS string) (*ControlPlane, *fakeAgentConn) {
	t.Helper()
	auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
	cp, err := NewControlPlane(Config{AuditPath: auditPath})
	if err != nil {
		t.Fatalf("new control plane: %v", err)
	}
	t.Cleanup(func() { _ = cp.Close() })

	conn := &fakeAgentConn{}
	cp.mu.Lock()
	cp.servers["srv"] = &Server{
		TenantID: "t1",
		ServerID: "srv",
		Status:   ServerOnline,
		OS:       serverOS,
	}
	cp.agentConns["srv"] = conn
	cp.mu.Unlock()
	return cp, conn
}

func TestCreateSession_DefaultsToChatOnWindowsWhenTypeEmpty(t *testing.T) {
	cp, conn := setupSessionTypeTestCP(t, "windows")
	sess, err := cp.CreateSession("ui:test", "t1", StartSessionRequest{
		ServerID: "srv",
		Cwd:      "/tmp",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if sess.SessionType != SessionTypeChat {
		t.Fatalf("expected chat session type, got %s", sess.SessionType)
	}
	if len(conn.msgs) == 0 {
		t.Fatal("expected message sent to agent")
	}
	if conn.msgs[0].Type != "start_chat" {
		t.Fatalf("expected start_chat, got %s", conn.msgs[0].Type)
	}
}

func TestCreateSession_DefaultsToPTYOnNonWindowsWhenTypeEmpty(t *testing.T) {
	cp, conn := setupSessionTypeTestCP(t, "linux")
	sess, err := cp.CreateSession("ui:test", "t1", StartSessionRequest{
		ServerID: "srv",
		Cwd:      "/tmp",
		Cols:     120,
		Rows:     30,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if sess.SessionType != SessionTypePTY {
		t.Fatalf("expected pty session type, got %s", sess.SessionType)
	}
	if len(conn.msgs) == 0 {
		t.Fatal("expected message sent to agent")
	}
	if conn.msgs[0].Type != "start_session" {
		t.Fatalf("expected start_session, got %s", conn.msgs[0].Type)
	}
}

func TestCreateSession_RejectsPTYOnWindows(t *testing.T) {
	cp, conn := setupSessionTypeTestCP(t, "windows")
	_, err := cp.CreateSession("ui:test", "t1", StartSessionRequest{
		ServerID:    "srv",
		SessionType: SessionTypePTY,
		Cwd:         "/tmp",
		Cols:        120,
		Rows:        30,
	})
	if err == nil {
		t.Fatal("expected pty on windows to be rejected")
	}
	if err.Error() != errPTYUnsupportedOnWindows {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conn.msgs) != 0 {
		t.Fatalf("expected no message sent to agent, got %d", len(conn.msgs))
	}

	cp.mu.RLock()
	sessionCount := len(cp.sessions)
	cp.mu.RUnlock()
	if sessionCount != 0 {
		t.Fatalf("expected no session to be created, got %d", sessionCount)
	}
}
