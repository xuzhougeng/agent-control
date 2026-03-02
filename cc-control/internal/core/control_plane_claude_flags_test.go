package core

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

type captureSender struct {
	msgs []Envelope
}

func (s *captureSender) Send(msg Envelope) error {
	s.msgs = append(s.msgs, msg)
	return nil
}

func TestSupportsClaudeSessionFlags(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		want bool
	}{
		{name: "claude", cmd: "claude", want: true},
		{name: "claude code", cmd: "claude-code", want: true},
		{name: "claude abs", cmd: "/usr/local/bin/Claude", want: true},
		{name: "claude windows", cmd: `C:\tools\claude.exe`, want: true},
		{name: "claude wrapper", cmd: "/opt/bin/claude-wrapper.sh", want: false},
		{name: "bash", cmd: "/usr/bin/bash", want: false},
		{name: "zsh", cmd: "zsh", want: false},
		{name: "empty", cmd: " ", want: false},
	}

	for _, tc := range cases {
		if got := supportsClaudeSessionFlags(tc.cmd); got != tc.want {
			t.Fatalf("%s: supportsClaudeSessionFlags(%q)=%v want=%v", tc.name, tc.cmd, got, tc.want)
		}
	}
}

func TestSendStartInstance_NonClaudePathSkipsSessionFlags(t *testing.T) {
	cp, err := NewControlPlane(Config{AuditPath: filepath.Join(t.TempDir(), "audit.jsonl")})
	if err != nil {
		t.Fatalf("new control plane: %v", err)
	}
	t.Cleanup(func() { _ = cp.Close() })

	sender := &captureSender{}
	server := &Server{ClaudePath: "/usr/bin/bash"}
	sess := &Session{SessionID: "s1", ServerID: "srv", SessionType: SessionTypePTY, Cwd: "/tmp"}
	inst := &RuntimeInstance{InstanceID: "inst-1", SessionType: SessionTypePTY}

	if err := cp.sendStartInstance(sender, server, sess, inst, map[string]string{"A": "b"}, 120, 30, true); err != nil {
		t.Fatalf("sendStartInstance: %v", err)
	}
	if len(sender.msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(sender.msgs))
	}
	var payload struct {
		Cmd []string `json:"cmd"`
	}
	if err := json.Unmarshal(sender.msgs[0].Data, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if len(payload.Cmd) != 1 || payload.Cmd[0] != "/usr/bin/bash" {
		t.Fatalf("expected cmd without session flags, got %#v", payload.Cmd)
	}
	if len(sess.Cmd) != 1 || sess.Cmd[0] != "/usr/bin/bash" {
		t.Fatalf("expected session cmd without session flags, got %#v", sess.Cmd)
	}
}

func TestSendStartInstance_ClaudePathIncludesSessionFlags(t *testing.T) {
	cp, err := NewControlPlane(Config{AuditPath: filepath.Join(t.TempDir(), "audit.jsonl")})
	if err != nil {
		t.Fatalf("new control plane: %v", err)
	}
	t.Cleanup(func() { _ = cp.Close() })

	sender := &captureSender{}
	server := &Server{ClaudePath: "claude-code"}
	sess := &Session{SessionID: "s1", ServerID: "srv", SessionType: SessionTypePTY, Cwd: "/tmp"}
	inst := &RuntimeInstance{InstanceID: "inst-1", SessionType: SessionTypePTY}

	if err := cp.sendStartInstance(sender, server, sess, inst, nil, 80, 24, false); err != nil {
		t.Fatalf("sendStartInstance: %v", err)
	}
	var payload struct {
		Cmd []string `json:"cmd"`
	}
	if err := json.Unmarshal(sender.msgs[0].Data, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if len(payload.Cmd) != 3 || payload.Cmd[1] != "--session-id" || payload.Cmd[2] != "s1" {
		t.Fatalf("expected claude session flag, got %#v", payload.Cmd)
	}
}
