package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cc-proxy/internal/claudecli"
)

func TestShouldRetryWithContinueFlag_AlreadyInUse(t *testing.T) {
	if !shouldRetryWithContinueFlag("Error: Session ID abc is already in use.") {
		t.Fatal("expected already-in-use stderr to trigger continue-session retry")
	}
}

func TestHandleMessageReturnsQuietlyWhenCanceled(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "slow-claude.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)

	sessionReady := false
	start := time.Now()
	reply, meta := handleMessage(ctx, claudecli.Config{Cmd: script, TimeoutMS: 60_000}, "s1", &sessionReady, "hello", nil, nil)
	if time.Since(start) > 2*time.Second {
		t.Fatal("handleMessage should stop quickly after cancellation")
	}
	if reply != "" {
		t.Fatalf("expected empty reply after cancellation, got %q", reply)
	}
	if len(meta) != 0 {
		t.Fatalf("expected no meta after cancellation, got %s", string(meta))
	}
	if sessionReady {
		t.Fatal("session should not be marked ready after cancellation")
	}
}

func TestShouldTerminateWorker(t *testing.T) {
	if shouldTerminateWorker(nil) {
		t.Fatal("nil context should not terminate worker")
	}
	if shouldTerminateWorker(context.Background()) {
		t.Fatal("background context should not terminate worker")
	}

	root := context.Background()
	child, cancelChild := context.WithCancel(root)
	cancelChild()
	if shouldTerminateWorker(root) {
		t.Fatal("canceling child context should not terminate worker")
	}
	if !shouldTerminateWorker(child) {
		t.Fatal("canceled context should terminate worker")
	}
}
