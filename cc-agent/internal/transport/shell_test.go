package transport

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecShell_Success(t *testing.T) {
	var live bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := execShell(ctx, "echo hello", "", &live, 2*time.Second, 1<<14)
	if err != nil {
		t.Fatalf("execShell: %v", err)
	}
	if !strings.Contains(got, "hello") {
		t.Errorf("captured output missing 'hello'; got=%q", got)
	}
	if !strings.Contains(live.String(), "hello") {
		t.Errorf("live stream missing 'hello'; got=%q", live.String())
	}
}

func TestExecShell_NonZeroExit(t *testing.T) {
	var live bytes.Buffer
	ctx := context.Background()
	got, err := execShell(ctx, "echo boom; exit 7", "", &live, 2*time.Second, 1<<14)
	if err != nil {
		t.Fatalf("execShell returned error; expected nil: %v", err)
	}
	if !strings.Contains(got, "boom") {
		t.Errorf("missing stdout in capture: %q", got)
	}
	if !strings.Contains(got, "[exit:") {
		t.Errorf("missing exit marker in capture: %q", got)
	}
}

func TestExecShell_Timeout(t *testing.T) {
	var live bytes.Buffer
	start := time.Now()
	got, err := execShell(context.Background(), "sleep 5", "", &live, 200*time.Millisecond, 1<<14)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("expected nil err; got %v", err)
	}
	if !strings.Contains(got, "[timed out]") {
		t.Errorf("missing timeout marker: %q", got)
	}
	// Without process-group kill, cmd.Run waits for the grandchild
	// `sleep` to close the inherited stdout pipe — taking ~5s instead
	// of ~200ms. Cap at 2s so we catch the regression but tolerate CI
	// jitter.
	if elapsed > 2*time.Second {
		t.Errorf("timeout returned slowly (%v); grandchild likely not killed", elapsed)
	}
}

func TestExecShell_Truncation(t *testing.T) {
	var live bytes.Buffer
	// Print 4KiB of 'x's; cap at 100 bytes.
	got, err := execShell(context.Background(), "printf 'x%.0s' $(seq 1 4096)", "", &live, 2*time.Second, 100)
	if err != nil {
		t.Fatalf("execShell: %v", err)
	}
	if !strings.Contains(got, "…(+") {
		t.Errorf("expected truncation marker; got=%q", got)
	}
	if !strings.Contains(got, "+3996 more bytes") {
		t.Errorf("expected exact dropped-bytes marker '+3996 more bytes'; got=%q", got)
	}
	if len(got) > 200 {
		t.Errorf("captured output should be truncated; got %d bytes", len(got))
	}
	if len(live.String()) < 4096 {
		t.Errorf("live stream should be uncapped; got %d bytes", len(live.String()))
	}
}

func TestExecShell_Cwd(t *testing.T) {
	var live bytes.Buffer
	tmp := t.TempDir()
	resolved, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", tmp, err)
	}
	got, err := execShell(context.Background(), "pwd", tmp, &live, 2*time.Second, 1<<14)
	if err != nil {
		t.Fatalf("execShell: %v", err)
	}
	if !strings.Contains(got, resolved) {
		t.Errorf("captured output missing cwd %q; got=%q", resolved, got)
	}
}

func TestPendingShell_PushTake(t *testing.T) {
	var p pendingShell
	if !p.empty() {
		t.Fatal("new buffer should be empty")
	}
	p.push("echo a", "a\n")
	p.push("echo b", "b\n")
	if p.empty() {
		t.Fatal("buffer should be non-empty")
	}
	got := p.take()
	if len(got) != 2 || got[0].cmd != "echo a" || got[1].cmd != "echo b" {
		t.Errorf("unexpected drain: %+v", got)
	}
	if !p.empty() {
		t.Fatal("take() should drain the buffer")
	}
}

func TestFoldShellContext_NoEntriesReturnsRawInput(t *testing.T) {
	got := foldShellContext(nil, "hello")
	if got != "hello" {
		t.Errorf("empty buffer should pass input through; got=%q", got)
	}
}

func TestFoldShellContext_OneEntry(t *testing.T) {
	entries := []shellEntry{{cmd: "ls", out: "foo\nbar\n"}}
	got := foldShellContext(entries, "fix it")
	want := "[user-shell] $ ls\nfoo\nbar\n===\nfix it"
	if got != want {
		t.Errorf("fold mismatch:\nwant=%q\ngot =%q", want, got)
	}
}

func TestFoldShellContext_MultipleEntriesAndEmptyOutput(t *testing.T) {
	entries := []shellEntry{
		{cmd: "true", out: ""},
		{cmd: "echo y", out: "y\n"},
	}
	got := foldShellContext(entries, "now what")
	want := "[user-shell] $ true\n(no output)\n---\n[user-shell] $ echo y\ny\n===\nnow what"
	if got != want {
		t.Errorf("fold mismatch:\nwant=%q\ngot =%q", want, got)
	}
}
