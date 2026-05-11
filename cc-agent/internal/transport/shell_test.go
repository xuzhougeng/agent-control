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

func TestPendingInject_PushTake(t *testing.T) {
	var p pendingInject
	if !p.empty() {
		t.Fatal("new buffer should be empty")
	}
	p.pushShell("echo a", "a\n")
	p.pushShell("echo b", "b\n")
	if p.empty() {
		t.Fatal("buffer should be non-empty")
	}
	got := p.take()
	if len(got) != 2 || got[0].head != "$ echo a" || got[1].head != "$ echo b" {
		t.Errorf("unexpected drain: %+v", got)
	}
	if !p.empty() {
		t.Fatal("take() should drain the buffer")
	}
}

func TestFoldInject_NoEntriesReturnsRawInput(t *testing.T) {
	got := foldInject(nil, "hello")
	if got != "hello" {
		t.Errorf("empty buffer should pass input through; got=%q", got)
	}
}

func TestFoldInject_OneShellEntry(t *testing.T) {
	entries := []injectEntry{{label: "user-shell", head: "$ ls", body: "foo\nbar\n"}}
	got := foldInject(entries, "fix it")
	want := "[user-shell] $ ls\nfoo\nbar\n===\nfix it"
	if got != want {
		t.Errorf("fold mismatch:\nwant=%q\ngot =%q", want, got)
	}
}

func TestFoldInject_MultipleShellEntriesAndEmptyOutput(t *testing.T) {
	entries := []injectEntry{
		{label: "user-shell", head: "$ true", body: ""},
		{label: "user-shell", head: "$ echo y", body: "y\n"},
	}
	got := foldInject(entries, "now what")
	want := "[user-shell] $ true\n(no output)\n---\n[user-shell] $ echo y\ny\n===\nnow what"
	if got != want {
		t.Errorf("fold mismatch:\nwant=%q\ngot =%q", want, got)
	}
}

func TestSplitAttachArgs(t *testing.T) {
	cases := []struct {
		in, path, prompt string
	}{
		{"foo.txt", "foo.txt", ""},
		{"foo.txt explain", "foo.txt", "explain"},
		{"foo.txt   what's wrong", "foo.txt", "what's wrong"},
		{"a/b.c\tinline-tab", "a/b.c", "inline-tab"},
	}
	for _, c := range cases {
		gotP, gotT := splitAttachArgs(c.in)
		if gotP != c.path || gotT != c.prompt {
			t.Errorf("split(%q) = (%q, %q); want (%q, %q)", c.in, gotP, gotT, c.path, c.prompt)
		}
	}
}

func TestFoldInject_AttachOnly(t *testing.T) {
	entries := []injectEntry{
		{label: labelAttach, head: "@/tmp/foo.txt", body: "hello\n"},
	}
	got := foldInject(entries, "summarize")
	want := "[user-attach] @/tmp/foo.txt\nhello\n===\nsummarize"
	if got != want {
		t.Errorf("fold mismatch:\nwant=%q\ngot =%q", want, got)
	}
}

func TestFoldInject_MixedShellAndAttach(t *testing.T) {
	entries := []injectEntry{
		{label: labelShell, head: "$ ls", body: "a\nb\n"},
		{label: labelAttach, head: "@/tmp/x.log", body: "ERROR: boom\n"},
	}
	got := foldInject(entries, "explain")
	want := "[user-shell] $ ls\na\nb\n---\n[user-attach] @/tmp/x.log\nERROR: boom\n===\nexplain"
	if got != want {
		t.Errorf("fold mismatch:\nwant=%q\ngot =%q", want, got)
	}
}
