package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCSV(t *testing.T) {
	got := ParseCSV(" a, ,b,, c ")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("unexpected csv parse result: %#v", got)
	}
}

func TestNormalizeRootsRequiresInput(t *testing.T) {
	_, err := NormalizeRoots(nil)
	if err == nil {
		t.Fatal("expected error for empty roots")
	}
}

func TestNormalizeRootsAndValidateCWD(t *testing.T) {
	root := t.TempDir()
	allowedChild := filepath.Join(root, "project")
	if err := os.MkdirAll(allowedChild, 0o755); err != nil {
		t.Fatalf("mkdir allowed child: %v", err)
	}

	roots, err := NormalizeRoots([]string{root})
	if err != nil {
		t.Fatalf("normalize roots: %v", err)
	}
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}

	if err := ValidateCWD(allowedChild, roots); err != nil {
		t.Fatalf("allowed cwd should pass, err=%v", err)
	}

	outside := t.TempDir()
	if err := ValidateCWD(outside, roots); err == nil {
		t.Fatal("outside cwd should be rejected")
	}
}

func TestFilterEnv(t *testing.T) {
	input := map[string]string{
		"CC_PROFILE": "dev",
		"KEEP_ME":    "1",
		"DROP_ME":    "x",
	}
	allowed := map[string]struct{}{
		"KEEP_ME": {},
	}
	got := FilterEnv(input, allowed, "CC_")
	if len(got) != 2 {
		t.Fatalf("expected 2 env keys, got %d (%#v)", len(got), got)
	}
	if got["CC_PROFILE"] != "dev" || got["KEEP_ME"] != "1" {
		t.Fatalf("unexpected filtered env: %#v", got)
	}
	if _, ok := got["DROP_ME"]; ok {
		t.Fatalf("DROP_ME should not pass filter: %#v", got)
	}
}
