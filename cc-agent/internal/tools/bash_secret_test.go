package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"cc-agent/internal/host"
	"cc-agent/internal/secrets"
)

func TestBashRejectsSecretLeak(t *testing.T) {
	b := NewBash(t.TempDir())
	out, err := b.Run(context.Background(), map[string]any{
		"command": "sudo -S cat /etc/shadow",
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "DENIED") || !strings.Contains(out, "secret leak") {
		t.Fatalf("expected secret-leak deny, got:\n%s", out)
	}
}

type denyPrompt struct{}

func (denyPrompt) Prompt(context.Context, secrets.Request) ([]byte, bool, error) {
	return nil, false, nil
}

func TestBashSudoDeniedWithoutTicket(t *testing.T) {
	b := NewBash(t.TempDir())
	b.sudoAvailable = func() bool { return true }
	b.sudoHasTicket = func() bool { return false }
	b.Book = secrets.NewBook(denyPrompt{})

	out, err := b.Run(context.Background(), map[string]any{
		"command": "sudo true",
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "DENIED") || !strings.Contains(out, "sudo") {
		t.Fatalf("expected sudo secret deny, got:\n%s", out)
	}
}

func TestBashSudoAskpassApplied(t *testing.T) {
	dir := t.TempDir()
	shellPath := filepath.Join(dir, "fake-shell")
	if err := os.WriteFile(shellPath, []byte("#!/bin/sh\nprintf '%s\\n' \"$SUDO_ASKPASS\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	b := NewBash(dir)
	b.Shell = host.Shell{Kind: host.ShellPOSIX, Command: shellPath}
	b.sudoAvailable = func() bool { return true }
	b.sudoHasTicket = func() bool { return false }
	b.Book = secrets.NewBook(&grantPrompt{secret: []byte("pw")})
	var applied bool
	b.startAskpass = func(secret []byte) (func(*exec.Cmd), func(), error) {
		if string(secret) != "pw" {
			t.Errorf("askpass secret = %q", secret)
		}
		return func(cmd *exec.Cmd) {
			applied = true
			if cmd.Env == nil {
				cmd.Env = os.Environ()
			}
			cmd.Env = append(cmd.Env, "SUDO_ASKPASS=/tmp/fake-askpass")
		}, func() {}, nil
	}

	out, err := b.Run(context.Background(), map[string]any{"command": "printf hi"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if applied {
		t.Fatalf("askpass should not run for non-sudo: %s", out)
	}

	applied = false
	out, err = b.Run(context.Background(), map[string]any{"command": "sudo printf hi"})
	if err != nil {
		t.Fatalf("sudo run: %v", err)
	}
	if !applied {
		t.Fatalf("askpass was not applied:\n%s", out)
	}
	if !strings.Contains(out, "/tmp/fake-askpass") {
		t.Fatalf("SUDO_ASKPASS not visible to command:\n%s", out)
	}
}

func TestBashSudoSkipsAskpassWhenTicketValid(t *testing.T) {
	dir := t.TempDir()
	shellPath := filepath.Join(dir, "fake-shell")
	if err := os.WriteFile(shellPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	b := NewBash(dir)
	b.Shell = host.Shell{Kind: host.ShellPOSIX, Command: shellPath}
	b.sudoAvailable = func() bool { return true }
	b.sudoHasTicket = func() bool { return true }
	called := false
	b.Book = secrets.NewBook(&grantPrompt{secret: []byte("pw")})
	b.startAskpass = func([]byte) (func(*exec.Cmd), func(), error) {
		called = true
		return nil, nil, nil
	}
	if _, err := b.Run(context.Background(), map[string]any{"command": "sudo true"}); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("askpass started despite valid sudo ticket")
	}
}

type grantPrompt struct{ secret []byte }

func (g *grantPrompt) Prompt(context.Context, secrets.Request) ([]byte, bool, error) {
	out := make([]byte, len(g.secret))
	copy(out, g.secret)
	return out, true, nil
}
