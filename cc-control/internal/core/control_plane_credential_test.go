package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleAgentCredentialRequest_ForwardsWithoutAuditingSecret(t *testing.T) {
	auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
	cp, err := NewControlPlane(Config{AuditPath: auditPath})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cp.Close() })

	conn := &fakeAgentConn{}
	sessionID := "s-cred"
	instanceID := "i-cred"
	tenantID := "t1"
	cp.mu.Lock()
	cp.servers["srv"] = &Server{TenantID: tenantID, ServerID: "srv", Status: ServerOnline}
	cp.agentConns["srv"] = conn
	cp.sessions[sessionID] = &Session{
		TenantID:         tenantID,
		SessionID:        sessionID,
		ServerID:         "srv",
		SessionType:      SessionTypeChat,
		ActiveInstanceID: instanceID,
		Status:           SessionRunning,
	}
	cp.instances[instanceID] = &RuntimeInstance{
		InstanceID:  instanceID,
		SessionID:   sessionID,
		TenantID:    tenantID,
		ServerID:    "srv",
		SessionType: SessionTypeChat,
		Status:      SessionRunning,
	}
	cp.mu.Unlock()

	cp.HandleAgentCredentialRequest("srv", sessionID, instanceID, "cr_1", "sudo", "sudo needs a password", "sudo apt update")

	pending := cp.GetPendingApprovalEvents(tenantID)
	if len(pending) != 1 {
		t.Fatalf("pending = %d, want 1", len(pending))
	}
	if pending[0].Kind != "credential_needed" {
		t.Fatalf("kind = %q", pending[0].Kind)
	}
	if pending[0].SecretName != "sudo" {
		t.Fatalf("secret_name = %q", pending[0].SecretName)
	}
	if strings.Contains(pending[0].PromptText, "hunter2") {
		t.Fatal("event leaked a secret")
	}

	const password = "hunter2-not-for-audit"
	if err := cp.HandleClientAction("ui:test", tenantID, sessionID, ActionRequest{
		Kind:    "credential_submit",
		EventID: pending[0].EventID,
		Secret:  password,
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}

	if len(conn.msgs) == 0 {
		t.Fatal("expected credential_decision")
	}
	msg := conn.msgs[len(conn.msgs)-1]
	if msg.Type != "credential_decision" {
		t.Fatalf("type = %q", msg.Type)
	}
	var payload map[string]any
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["granted"] != true {
		t.Fatalf("granted = %v", payload["granted"])
	}
	if payload["secret"] != password {
		t.Fatalf("agent should receive the secret")
	}

	raw, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), password) {
		t.Fatalf("audit log contained the password:\n%s", raw)
	}
	if !strings.Contains(string(raw), "credential_needed") {
		t.Fatalf("audit missing credential_needed:\n%s", raw)
	}
}

func TestHandleClientAction_ApproveOnCredentialRequiresPassword(t *testing.T) {
	auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
	cp, err := NewControlPlane(Config{AuditPath: auditPath})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cp.Close() })

	conn := &fakeAgentConn{}
	sessionID := "s1"
	instanceID := "i1"
	eventID := "e1"
	cp.mu.Lock()
	cp.servers["srv"] = &Server{TenantID: "t1", ServerID: "srv", Status: ServerOnline}
	cp.agentConns["srv"] = conn
	cp.sessions[sessionID] = &Session{
		TenantID: "t1", SessionID: sessionID, ServerID: "srv",
		ActiveInstanceID: instanceID, AwaitingApproval: true, PendingEventID: eventID,
		Status: SessionRunning, SessionType: SessionTypeChat,
	}
	cp.instances[instanceID] = &RuntimeInstance{
		InstanceID: instanceID, SessionID: sessionID, ServerID: "srv", TenantID: "t1",
		AwaitingApproval: true, PendingEventID: eventID, Status: SessionRunning,
	}
	cp.sessionEvents[sessionID] = []SessionEvent{{
		EventID: eventID, SessionID: sessionID, InstanceID: instanceID, ServerID: "srv",
		TenantID: "t1", Kind: "credential_needed", AgentRequestID: "cr_1", SecretName: "mysql/prod",
	}}
	cp.mu.Unlock()

	err = cp.HandleClientAction("ui:test", "t1", sessionID, ActionRequest{Kind: "approve", EventID: eventID})
	if err == nil || !strings.Contains(err.Error(), "password required") {
		t.Fatalf("err = %v, want password required", err)
	}
}
