package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"cc-agent/internal/security"
)

func TestRegisterPayloadReturnsDefensiveCopies(t *testing.T) {
	mgr := NewSessionManager(Config{
		ServerID:   "srv-test",
		Hostname:   "host-test",
		Tags:       []string{"a", "b"},
		AllowRoots: []string{"/tmp/root-a", "/tmp/root-b"},
		ClaudePath: "/bin/sh",
	})

	p1 := mgr.RegisterPayload()
	p1.Tags[0] = "mutated-tag"
	p1.AllowRoots[0] = "/mutated/root"

	p2 := mgr.RegisterPayload()
	if p2.Tags[0] != "a" {
		t.Fatalf("tags should not be mutated across calls: %#v", p2.Tags)
	}
	if p2.AllowRoots[0] != "/tmp/root-a" {
		t.Fatalf("allow roots should not be mutated across calls: %#v", p2.AllowRoots)
	}
}

func TestStartSessionRejectsResumeIDWhitespaceAndSendsError(t *testing.T) {
	root := t.TempDir()
	roots, err := security.NormalizeRoots([]string{root})
	if err != nil {
		t.Fatalf("normalize roots: %v", err)
	}
	mgr := NewSessionManager(Config{
		ServerID:       "srv-test",
		AllowRoots:     roots,
		ClaudePath:     "/bin/sh",
		EnvAllowPrefix: "CC_",
	})

	var sent []Envelope
	mgr.SetSendFunc(func(msg Envelope) error {
		sent = append(sent, msg)
		return nil
	})

	err = mgr.startSession("s1", StartSessionPayload{
		Cwd:      root,
		ResumeID: "bad id",
		Cols:     120,
		Rows:     30,
	})
	if err == nil {
		t.Fatal("expected whitespace resume_id to be rejected")
	}
	if !strings.Contains(err.Error(), "contains whitespace") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sent) == 0 {
		t.Fatal("expected an error message to be sent to control plane")
	}
	if sent[0].Type != "error" {
		t.Fatalf("expected first sent envelope type=error, got %q", sent[0].Type)
	}
	var payload struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(sent[0].Data, &payload); err != nil {
		t.Fatalf("decode sent error payload: %v", err)
	}
	if !strings.Contains(payload.Message, "reject_resume_id:contains_whitespace") {
		t.Fatalf("unexpected error payload message: %q", payload.Message)
	}
}

func TestStartSessionMissingSessionID(t *testing.T) {
	root := t.TempDir()
	mgr := NewSessionManager(Config{
		ServerID:   "srv-test",
		AllowRoots: []string{root},
		ClaudePath: "/bin/sh",
	})

	err := mgr.startSession("", StartSessionPayload{Cwd: root})
	if err == nil {
		t.Fatal("expected missing session_id error")
	}
	if !strings.Contains(err.Error(), "missing session_id") {
		t.Fatalf("unexpected error: %v", err)
	}
}
