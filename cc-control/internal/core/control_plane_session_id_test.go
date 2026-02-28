package core

import (
	"path/filepath"
	"testing"
)

func setupSessionIDTestCP(t *testing.T) (*ControlPlane, *fakeAgentConn) {
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
		OS:       "linux",
	}
	cp.agentConns["srv"] = conn
	cp.mu.Unlock()
	return cp, conn
}

func TestCreateSession_WithProvidedSessionID(t *testing.T) {
	cp, conn := setupSessionIDTestCP(t)

	wantSessionID := "11111111-1111-1111-1111-111111111111"
	sess, err := cp.CreateSession("ui:test", "t1", StartSessionRequest{
		SessionID: wantSessionID,
		ServerID:  "srv",
		Cwd:       "/tmp",
		Cols:      120,
		Rows:      30,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if sess.SessionID != wantSessionID {
		t.Fatalf("expected session_id=%s, got %s", wantSessionID, sess.SessionID)
	}
	if sess.ActiveInstanceID == "" {
		t.Fatal("expected active_instance_id to be set")
	}
	if len(conn.msgs) == 0 {
		t.Fatal("expected message sent to agent")
	}
	if conn.msgs[0].SessionID != wantSessionID {
		t.Fatalf("expected envelope session_id=%s, got %s", wantSessionID, conn.msgs[0].SessionID)
	}
	if conn.msgs[0].InstanceID != sess.ActiveInstanceID {
		t.Fatalf("expected envelope instance_id=%s, got %s", sess.ActiveInstanceID, conn.msgs[0].InstanceID)
	}
}

func TestCreateSession_InvalidSessionIDRejected(t *testing.T) {
	cp, conn := setupSessionIDTestCP(t)

	_, err := cp.CreateSession("ui:test", "t1", StartSessionRequest{
		SessionID: "bad-id",
		ServerID:  "srv",
		Cwd:       "/tmp",
		Cols:      120,
		Rows:      30,
	})
	if err == nil {
		t.Fatal("expected invalid session_id error")
	}
	if err.Error() != "invalid session_id" {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conn.msgs) != 0 {
		t.Fatalf("expected no message sent to agent, got %d", len(conn.msgs))
	}
}

func TestCreateSession_DuplicateSessionIDRejected(t *testing.T) {
	cp, _ := setupSessionIDTestCP(t)

	sessionID := "22222222-2222-2222-2222-222222222222"
	_, err := cp.CreateSession("ui:test", "t1", StartSessionRequest{
		SessionID: sessionID,
		ServerID:  "srv",
		Cwd:       "/tmp",
		Cols:      120,
		Rows:      30,
	})
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}

	_, err = cp.CreateSession("ui:test", "t1", StartSessionRequest{
		SessionID: sessionID,
		ServerID:  "srv",
		Cwd:       "/tmp",
		Cols:      120,
		Rows:      30,
	})
	if err == nil {
		t.Fatal("expected duplicate session_id error")
	}
	if err.Error() != "session_id already exists" {
		t.Fatalf("unexpected error: %v", err)
	}
}
