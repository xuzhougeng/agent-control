package echocli

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

func echoCmd() (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "findstr", "^"}
	}
	return "cat", nil
}

func TestSession_WriteAndRead(t *testing.T) {
	cmd, args := echoCmd()
	received := make(chan Message, 1)
	exited := make(chan struct{})

	sess, err := Start("test-1", os.TempDir(), cmd, args, nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	sess.SetCallbacks(func(msg Message) {
		received <- msg
	}, func(code *int, reason string) {
		close(exited)
	})

	err = sess.Write(Message{MessageID: "m1", Content: "hello"})
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case msg := <-received:
		if msg.MessageID != "m1" || msg.Content != "hello" {
			t.Fatalf("unexpected message: %+v", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for response")
	}

	sess.Stop(0, 0)
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for exit")
	}
}

func TestSession_StopCausesExit(t *testing.T) {
	cmd, args := echoCmd()
	exited := make(chan struct{})

	sess, err := Start("test-2", os.TempDir(), cmd, args, nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	sess.SetCallbacks(nil, func(code *int, reason string) {
		close(exited)
	})

	sess.Stop(0, 0)
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for exit")
	}

	if sess.IsRunning() {
		t.Fatal("session should not be running after stop")
	}
}

func TestSession_StopKillsChildProcessTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group kill test is unix-only")
	}

	tmp := t.TempDir()
	pidFile := filepath.Join(tmp, "child.pid")
	cmd := "sh"
	args := []string{"-c", "sleep 30 & echo $! > \"$PID_FILE\"; cat"}
	exited := make(chan struct{})

	t.Setenv("PID_FILE", pidFile)
	sess, err := Start("test-3", tmp, cmd, args, nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	sess.SetCallbacks(nil, func(code *int, reason string) {
		close(exited)
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
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for exit")
	}

	proc, err := os.FindProcess(childPID)
	if err != nil {
		t.Fatalf("find child process: %v", err)
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		t.Fatalf("child process %d is still alive after session stop", childPID)
	}
}
