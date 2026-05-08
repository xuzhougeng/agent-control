package skills

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cc-agent/internal/llm"
)

// stubProvider returns a canned skill JSON regardless of the request.
type stubProvider struct{ canned string }

func (s *stubProvider) Name() string { return "stub" }
func (s *stubProvider) Complete(_ context.Context, _ llm.Request) (*llm.Response, error) {
	return &llm.Response{
		StopReason: llm.StopEnd,
		Message:    llm.Message{Role: llm.RoleAssistant, Content: s.canned},
	}, nil
}

func TestParseSkillJSON_StripsCodeFences(t *testing.T) {
	cases := []string{
		`{"name":"x","description":"y"}`,
		"```json\n{\"name\":\"x\",\"description\":\"y\"}\n```",
		"sure, here:\n```\n{\"name\":\"x\",\"description\":\"y\"}\n```\nhope this helps!",
	}
	for _, c := range cases {
		s, err := parseSkillJSON(c)
		if err != nil {
			t.Errorf("parse %q: %v", c, err)
			continue
		}
		if s.Name != "x" || s.Description != "y" {
			t.Errorf("parse %q -> %+v", c, s)
		}
	}
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"Nginx Triage!":    "nginx-triage",
		"   Disk-Space  ":  "disk-space",
		"a/b\\c":           "a-b-c",
		"":                 "",
		"--leading--trailing--": "leading--trailing",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUniqueToolNames(t *testing.T) {
	in := []TranscriptItem{
		{Role: "assistant", ToolName: "bash"},
		{Role: "tool"},
		{Role: "assistant", ToolName: "read"},
		{Role: "assistant", ToolName: "bash"},
		{Role: "user"},
	}
	got := uniqueToolNames(in)
	want := []string{"bash", "read"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestLLMReflector_DistillAndSave(t *testing.T) {
	canned := `{
		"name": "kernel-probe",
		"description": "Identify the running kernel and OS",
		"prompt": "When asked about the system, run uname -r first, then read /etc/os-release. Summarize as a short table.",
		"examples": ["what kernel are we running?", "what OS is this server?"]
	}`
	r := &LLMReflector{Provider: &stubProvider{canned: canned}, Model: "stub"}

	in := DistillInput{
		History: []TranscriptItem{
			{Role: "user", Text: "what kernel are we running?"},
			{Role: "assistant", ToolName: "bash", ToolArgs: "{command=uname -r}"},
			{Role: "tool", Text: "6.6.87.2-microsoft-standard-WSL2"},
			{Role: "assistant", Text: "Kernel: 6.6.87.2 (WSL2)."},
		},
	}
	skill, err := r.Distill(context.Background(), in)
	if err != nil {
		t.Fatalf("distill: %v", err)
	}
	if skill.Name != "kernel-probe" {
		t.Errorf("name = %q", skill.Name)
	}
	// Tools were not in the canned response, so we expect them filled from
	// the transcript scan.
	if len(skill.Tools) != 1 || skill.Tools[0] != "bash" {
		t.Errorf("tools = %v, want [bash]", skill.Tools)
	}

	dir := t.TempDir()
	reg := NewRegistry()
	path, err := reg.Save(skill, dir)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if filepath.Dir(path) != dir {
		t.Errorf("save path %q not under dir %q", path, dir)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var roundTrip Skill
	if err := json.Unmarshal(raw, &roundTrip); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if roundTrip.Name != "kernel-probe" {
		t.Errorf("roundtrip name = %q", roundTrip.Name)
	}
	if got, ok := reg.Get("kernel-probe"); !ok || got != skill {
		t.Errorf("registry missing skill after save")
	}
}

func TestLLMReflector_RejectsEmptyHistory(t *testing.T) {
	r := &LLMReflector{Provider: &stubProvider{canned: "{}"}, Model: "x"}
	if _, err := r.Distill(context.Background(), DistillInput{}); err == nil {
		t.Errorf("expected error on empty history")
	}
}

func TestLLMReflector_OperatorOverridesName(t *testing.T) {
	canned := `{"name":"auto-named","description":"auto","prompt":"x","examples":[]}`
	r := &LLMReflector{Provider: &stubProvider{canned: canned}, Model: "x"}
	in := DistillInput{
		Name:    "Operator Picked",
		History: []TranscriptItem{{Role: "user", Text: "hi"}},
	}
	skill, err := r.Distill(context.Background(), in)
	if err != nil {
		t.Fatalf("distill: %v", err)
	}
	if skill.Name != "operator-picked" {
		t.Errorf("operator name should win after sanitize, got %q", skill.Name)
	}
}
