package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTwoPhase_TeamOverridesLocal(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, "local")
	team := filepath.Join(root, "team")
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(team, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "x.json"), []byte(`{"name":"x","prompt":"local"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(team, "x.json"), []byte(`{"name":"x","prompt":"team"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry()
	if err := LoadTwoPhase(r, local, team); err != nil {
		t.Fatalf("LoadTwoPhase: %v", err)
	}
	got, ok := r.Get("x")
	if !ok {
		t.Fatalf("not loaded")
	}
	if got.Prompt != "team" {
		t.Fatalf("Prompt=%q, want team", got.Prompt)
	}
}

func TestLoadTwoPhase_LocalOnlyWhenTeamMissing(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, "local")
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "x.json"), []byte(`{"name":"x","prompt":"local"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry()
	if err := LoadTwoPhase(r, local, filepath.Join(root, "team")); err != nil {
		t.Fatalf("LoadTwoPhase: %v", err)
	}
	got, _ := r.Get("x")
	if got.Prompt != "local" {
		t.Fatalf("Prompt=%q, want local", got.Prompt)
	}
}
