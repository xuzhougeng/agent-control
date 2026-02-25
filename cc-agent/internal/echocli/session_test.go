package echocli

import (
	"os"
	"runtime"
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

	sess.Stop()
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

	sess.Stop()
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for exit")
	}

	if sess.IsRunning() {
		t.Fatal("session should not be running after stop")
	}
}
