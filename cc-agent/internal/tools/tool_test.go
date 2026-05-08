package tools

import (
	"context"
	"strings"
	"testing"
)

func TestDefaultRegistryHasOpsTools(t *testing.T) {
	r := DefaultRegistry("/tmp", nil)
	want := []string{"bash", "read", "write", "grep", "glob", "sysinfo", "proclist", "logtail"}
	for _, n := range want {
		if _, ok := r.Get(n); !ok {
			t.Errorf("default registry missing %q", n)
		}
	}
}

func TestBashRunsAndReportsExit(t *testing.T) {
	b := NewBash("/tmp")
	out, err := b.Run(context.Background(), map[string]any{"command": "printf hi"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "exit_code: 0") {
		t.Errorf("missing exit_code: %s", out)
	}
	if !strings.Contains(out, "hi") {
		t.Errorf("missing stdout: %s", out)
	}
}

func TestBashTimeoutCapped(t *testing.T) {
	b := NewBash("/tmp")
	out, err := b.Run(context.Background(), map[string]any{
		"command":     "sleep 5",
		"timeout_sec": 1,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "timed_out: true") {
		t.Errorf("expected timed_out true: %s", out)
	}
}

func TestReadWriteRoundtrip(t *testing.T) {
	dir := t.TempDir()
	w := NewWrite(dir)
	if _, err := w.Run(context.Background(), map[string]any{
		"path":    "hello.txt",
		"content": "world",
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	r := NewRead(dir)
	out, err := r.Run(context.Background(), map[string]any{"path": "hello.txt"})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if out != "world" {
		t.Errorf("roundtrip mismatch: %q", out)
	}
}

func TestWriteRefusesOverwriteByDefault(t *testing.T) {
	dir := t.TempDir()
	w := NewWrite(dir)
	in := map[string]any{"path": "x.txt", "content": "v1"}
	if _, err := w.Run(context.Background(), in); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if _, err := w.Run(context.Background(), in); err == nil {
		t.Errorf("expected refusal on overwrite")
	}
	in["overwrite"] = true
	if _, err := w.Run(context.Background(), in); err != nil {
		t.Errorf("overwrite=true should succeed: %v", err)
	}
}
