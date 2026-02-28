package pty

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSessionStopKillsChildProcessTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group kill test is unix-only")
	}

	tmp := t.TempDir()
	pidFile := filepath.Join(tmp, "child.pid")
	t.Setenv("PID_FILE", pidFile)

	sess, err := Start(
		"pty-1",
		tmp,
		"sh",
		[]string{"-c", "sleep 30 & echo $! > \"$PID_FILE\"; cat"},
		nil,
		120,
		30,
	)
	if err != nil {
		if os.IsPermission(err) || strings.Contains(strings.ToLower(err.Error()), "operation not permitted") {
			t.Skipf("pty start not permitted in this environment: %v", err)
		}
		t.Fatalf("start: %v", err)
	}

	exitCh := make(chan struct{})
	go sess.ReadLoop(func(seq uint64, chunk []byte) {}, func(code *int, signal, reason string) {
		close(exitCh)
	})

	deadline := time.Now().Add(2 * time.Second)
	var childPID int
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(pidFile)
		if err == nil {
			n, convErr := strconv.Atoi(strings.TrimSpace(string(raw)))
			if convErr == nil && n > 0 {
				childPID = n
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	if childPID == 0 {
		t.Fatal("child pid file was not written")
	}

	sess.Stop(0, 0)
	select {
	case <-exitCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for pty exit")
	}

	proc, err := os.FindProcess(childPID)
	if err != nil {
		t.Fatalf("find child process: %v", err)
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		t.Fatalf("child process %d is still alive after stop", childPID)
	}
}
