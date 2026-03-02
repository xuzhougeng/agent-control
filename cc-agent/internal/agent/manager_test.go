package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func TestStartSessionMissingSessionID(t *testing.T) {
	root := t.TempDir()
	mgr := NewSessionManager(Config{
		ServerID:   "srv-test",
		AllowRoots: []string{root},
		ClaudePath: "/bin/sh",
	})

	err := mgr.startSession("", "inst-1", StartSessionPayload{Cwd: root})
	if err == nil {
		t.Fatal("expected missing session_id error")
	}
	if !strings.Contains(err.Error(), "missing session_id or instance_id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartSessionRejectsExistingChatSession(t *testing.T) {
	root := t.TempDir()
	mgr := NewSessionManager(Config{
		ServerID:   "srv-test",
		AllowRoots: []string{root},
		ClaudePath: "/bin/sh",
	})
	mgr.chatSessions["inst-1"] = nil

	err := mgr.startSession("s1", "inst-1", StartSessionPayload{Cwd: root})
	if err == nil {
		t.Fatal("expected duplicate session error")
	}
	if !strings.Contains(err.Error(), "session already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartSessionWindowsRejectsPTYAndSendsError(t *testing.T) {
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

	origGOOS := runtimeGOOS
	runtimeGOOS = "windows"
	defer func() {
		runtimeGOOS = origGOOS
	}()

	var sent []Envelope
	mgr.SetSendFunc(func(msg Envelope) error {
		sent = append(sent, msg)
		return nil
	})

	err = mgr.startSession("s1", "inst-1", StartSessionPayload{
		Cwd: root,
	})
	if err == nil {
		t.Fatal("expected windows pty start to fail")
	}
	if !strings.Contains(err.Error(), ptyUnsupportedOnWindowsMessage) {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sent) == 0 {
		t.Fatal("expected an error envelope to be sent")
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
	wantMessage := "start_failed:" + ptyUnsupportedOnWindowsMessage
	if payload.Message != wantMessage {
		t.Fatalf("unexpected error payload message: %q", payload.Message)
	}
}

func TestNormalizeClaudeSessionArgsSwitchesToResumeWhenSessionEnvExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sessionID := "ba3a661b-5036-4616-885f-ad6e9d4d0f34"
	path := filepath.Join(home, ".claude", "session-env")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir session-env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, sessionID), []byte("1"), 0o644); err != nil {
		t.Fatalf("write session-env marker: %v", err)
	}

	args := normalizeClaudeSessionArgs(sessionID, []string{"--session-id", sessionID})
	if len(args) != 2 || args[0] != "--resume" || args[1] != sessionID {
		t.Fatalf("expected --resume %s, got %#v", sessionID, args)
	}
}

func TestNormalizeClaudeSessionArgsKeepsResumeWhenResumeTargetMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sessionID := "ba3a661b-5036-4616-885f-ad6e9d4d0f34"

	args := normalizeClaudeSessionArgs(sessionID, []string{"--resume", sessionID})
	if len(args) != 2 || args[0] != "--resume" || args[1] != sessionID {
		t.Fatalf("expected --resume %s, got %#v", sessionID, args)
	}
}

func TestAlternateClaudeSessionArgsSwitchesSessionIDToResume(t *testing.T) {
	sessionID := "ba3a661b-5036-4616-885f-ad6e9d4d0f34"
	alt, ok := alternateClaudeSessionArgs(sessionID, []string{"--session-id", sessionID})
	if !ok {
		t.Fatal("expected alternate args for --session-id")
	}
	if len(alt) != 2 || alt[0] != "--resume" || alt[1] != sessionID {
		t.Fatalf("expected --resume %s, got %#v", sessionID, alt)
	}
}

func TestAlternateClaudeSessionArgsSwitchesResumeToSessionID(t *testing.T) {
	sessionID := "ba3a661b-5036-4616-885f-ad6e9d4d0f34"
	alt, ok := alternateClaudeSessionArgs(sessionID, []string{"--resume", sessionID})
	if !ok {
		t.Fatal("expected alternate args for --resume")
	}
	if len(alt) != 2 || alt[0] != "--session-id" || alt[1] != sessionID {
		t.Fatalf("expected --session-id %s, got %#v", sessionID, alt)
	}
}

func TestAlternateClaudeSessionArgsNoSessionFlag(t *testing.T) {
	sessionID := "ba3a661b-5036-4616-885f-ad6e9d4d0f34"
	if _, ok := alternateClaudeSessionArgs(sessionID, []string{"-p"}); ok {
		t.Fatal("did not expect alternate args without session flag")
	}
}

func TestStripClaudeSessionArgs(t *testing.T) {
	in := []string{
		"--session-id", "s1",
		"-p",
		"--resume=s2",
		"--model", "sonnet",
		"--resume", "s3",
	}
	got := stripClaudeSessionArgs(in)
	want := []string{"-p", "--model", "sonnet"}
	if len(got) != len(want) {
		t.Fatalf("unexpected args length: got=%#v want=%#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected args: got=%#v want=%#v", got, want)
		}
	}
}

func TestShouldRetryClaudeSessionStartup(t *testing.T) {
	code := 1
	cases := []struct {
		name   string
		code   *int
		reason string
		output string
		want   bool
	}{
		{
			name:   "already in use output",
			code:   &code,
			reason: "exited",
			output: "Error: Session ID abc is already in use.",
			want:   true,
		},
		{
			name:   "no conversation output",
			code:   &code,
			reason: "exited",
			output: "Error: No conversation found with session ID abc.",
			want:   true,
		},
		{
			name:   "success exit code",
			code:   func() *int { v := 0; return &v }(),
			reason: "exited",
			output: "Error: Session ID abc is already in use.",
			want:   false,
		},
		{
			name:   "irrelevant error",
			code:   &code,
			reason: "exited",
			output: "permission denied",
			want:   false,
		},
	}

	for _, tc := range cases {
		if got := shouldRetryClaudeSessionStartup(tc.code, tc.reason, tc.output); got != tc.want {
			t.Fatalf("%s: expected %v, got %v", tc.name, tc.want, got)
		}
	}
}

func TestAppendRecentTextTrimsToLimit(t *testing.T) {
	got := appendRecentText("abcdef", []byte("ghij"), 8)
	if got != "cdefghij" {
		t.Fatalf("expected trimmed tail, got %q", got)
	}
}
