package control

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"cc-agent/internal/skills"
)

func TestWriteInstalledSkill_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	sk := &skills.StoredSkill{
		Skill:          skills.Skill{Name: "demo", Prompt: "hello", Tools: []string{"bash"}},
		Version:        5,
		AuthorServerID: "ops-X",
	}
	if err := writeInstalledSkill(dir, sk); err != nil {
		t.Fatalf("writeInstalledSkill: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "demo.json"))
	if err != nil {
		t.Fatalf("read installed: %v", err)
	}
	var got skills.Skill
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode installed: %v", err)
	}
	if got.Name != "demo" || got.Prompt != "hello" {
		t.Fatalf("got %+v", got)
	}
	// No leftover .tmp file from the rename.
	if _, err := os.Stat(filepath.Join(dir, "demo.json.tmp")); !os.IsNotExist(err) {
		t.Fatalf("expected no .tmp leftover, got err=%v", err)
	}
	// Idempotent: second write succeeds (overwrites).
	if err := writeInstalledSkill(dir, sk); err != nil {
		t.Fatalf("second writeInstalledSkill: %v", err)
	}
}

func TestWriteInstalledSkill_RejectsMissingTeamDir(t *testing.T) {
	sk := &skills.StoredSkill{Skill: skills.Skill{Name: "demo", Prompt: "p"}}
	if err := writeInstalledSkill("", sk); err == nil {
		t.Fatalf("expected error on empty teamDir")
	}
}

func TestWriteInstalledSkill_RejectsEmptyName(t *testing.T) {
	sk := &skills.StoredSkill{Skill: skills.Skill{Prompt: "p"}}
	if err := writeInstalledSkill(t.TempDir(), sk); err == nil {
		t.Fatalf("expected error on empty name")
	}
}

func TestWriteInstalledSkill_RejectsBadName(t *testing.T) {
	dir := t.TempDir()
	bad := []string{
		"../etc/passwd",
		"foo/bar",
		"foo..bar",
		".hidden",
		"UPPER",
		"foo bar",
		"",
		"x", // too short (regex requires len>=2)
	}
	for _, name := range bad {
		sk := &skills.StoredSkill{Skill: skills.Skill{Name: name, Prompt: "p"}}
		if err := writeInstalledSkill(dir, sk); err == nil {
			t.Fatalf("expected error for name %q", name)
		}
	}
	// Nothing leaked into the team dir.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("dir entries after rejected installs: %+v", entries)
	}
}

func TestInstallSkillRequest_NoTeamDirIsError(t *testing.T) {
	// installSkillRequest is the WS handler; with TeamDir empty it propagates
	// the writeInstalledSkill error rather than silently succeeding, so
	// operators see the misconfiguration in cc-control logs.
	c := &Client{opts: Options{TeamDir: ""}}
	payload, _ := json.Marshal(map[string]any{"name": "demo", "prompt": "p"})
	env := Envelope{Type: "install_skill_request", Data: payload}
	if err := c.installSkillRequest(env); err == nil {
		t.Fatalf("expected error when TeamDir empty")
	}
}
