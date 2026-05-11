package transport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadAttach_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, []byte("hi there\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, trunc, err := readAttach(path, dir)
	if err != nil {
		t.Fatalf("readAttach: %v", err)
	}
	if got != "hi there\n" {
		t.Errorf("content mismatch: %q", got)
	}
	if trunc {
		t.Errorf("unexpected truncation")
	}
}

func TestReadAttach_RelativePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rel.txt"), []byte("rel"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _, err := readAttach("rel.txt", dir)
	if err != nil {
		t.Fatalf("readAttach: %v", err)
	}
	if got != "rel" {
		t.Errorf("got %q", got)
	}
}

func TestReadAttach_Missing(t *testing.T) {
	_, _, err := readAttach("/no/such/file/here.txt", "")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadAttach_Directory(t *testing.T) {
	dir := t.TempDir()
	_, _, err := readAttach(dir, "")
	if err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("expected directory error; got %v", err)
	}
}

func TestReadAttach_Binary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bin.dat")
	// 4 KiB of bytes including invalid UTF-8 sequences.
	buf := make([]byte, 4096)
	for i := range buf {
		buf[i] = 0xFE
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := readAttach(path, "")
	if err == nil || !strings.Contains(err.Error(), "binary") {
		t.Fatalf("expected binary-rejection error; got %v", err)
	}
}

func TestReadAttach_Truncation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	big := strings.Repeat("a", 300*1024) // 300 KiB, above the 256 KiB cap
	if err := os.WriteFile(path, []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	got, trunc, err := readAttach(path, "")
	if err != nil {
		t.Fatalf("readAttach: %v", err)
	}
	if !trunc {
		t.Errorf("expected truncated=true")
	}
	if !strings.Contains(got, "…(+") {
		t.Errorf("missing truncation marker: ends with %q", got[len(got)-80:])
	}
	if !strings.Contains(got, "…(+45056 more bytes, truncated)") {
		t.Errorf("expected exact truncation marker '+45056 more bytes, truncated'; got tail=%q", got[len(got)-80:])
	}
}

func TestReadAttach_Symlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(real, []byte("via-symlink\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(real, link); err != nil {
		// Skip rather than fail on filesystems that disallow symlinks
		// (rare on Linux/macOS; commonly hit on a Windows test runner).
		t.Skipf("symlink not supported: %v", err)
	}
	got, trunc, err := readAttach(link, "")
	if err != nil {
		t.Fatalf("readAttach via symlink: %v", err)
	}
	if got != "via-symlink\n" {
		t.Errorf("symlink target not followed; got %q", got)
	}
	if trunc {
		t.Errorf("unexpected truncation")
	}
}

func TestReadAttach_TildeExpansion(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	path := filepath.Join(tmpHome, "tilde-test.txt")
	if err := os.WriteFile(path, []byte("home"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _, err := readAttach("~/tilde-test.txt", "")
	if err != nil {
		t.Fatalf("readAttach with ~: %v", err)
	}
	if got != "home" {
		t.Errorf("got %q", got)
	}
}
