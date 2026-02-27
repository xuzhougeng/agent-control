package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateExecutablePath_EmptyOK(t *testing.T) {
	got, err := resolveExecutablePath("")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty result, got %q", got)
	}
}

func TestValidateExecutablePath_RelativeFile(t *testing.T) {
	dir := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	name := "worker"
	if runtime.GOOS == "windows" {
		name = "worker.exe"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("echo"), 0o755); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o755); err != nil {
			t.Fatalf("chmod: %v", err)
		}
	}

	got, err := resolveExecutablePath("./" + name)
	if err != nil {
		t.Fatalf("expected valid relative path, got %v", err)
	}
	if got != path {
		t.Fatalf("expected resolved path %q, got %q", path, got)
	}
}

func TestValidateExecutablePath_Missing(t *testing.T) {
	_, err := resolveExecutablePath("./this/does/not/exist")
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}
