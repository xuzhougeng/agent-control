package skills

import (
	"context"
	"errors"
	"strings"
	"testing"

	"cc-agent/internal/llm"
)

func TestParseRouteDecision(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"strict", `{"skill":"nginx-triage"}`, "nginx-triage"},
		{"with prose", `Sure, I'll pick: {"skill": "kernel-probe"} done.`, "kernel-probe"},
		{"fenced", "```json\n{\"skill\": \"nginx-triage\"}\n```", "nginx-triage"},
		{"fenced no lang", "```\n{\"skill\":\"nginx-triage\"}\n```", "nginx-triage"},
		{"null", `{"skill": null}`, ""},
		{"trim spaces", `{"skill": "  nginx-triage  "}`, "nginx-triage"},
		{"empty obj", `{}`, ""},
		{"garbage", `not json at all`, ""},
		{"missing quote", `{"skill": nginx}`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseRouteDecision(c.in)
			if got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestWrapUserInput(t *testing.T) {
	bare := "nginx 502 怎么办"
	if got := WrapUserInput(bare, nil); got != bare {
		t.Errorf("nil skill: should pass through, got %q", got)
	}
	if got := WrapUserInput(bare, &Skill{Name: "x", Prompt: ""}); got != bare {
		t.Errorf("empty prompt: should pass through, got %q", got)
	}
	if got := WrapUserInput(bare, &Skill{Name: "x", Prompt: "   \t\n"}); got != bare {
		t.Errorf("whitespace-only prompt: should pass through, got %q", got)
	}
	skill := &Skill{Name: "nginx-triage", Prompt: "Focus on access.log"}
	got := WrapUserInput(bare, skill)
	if !strings.Contains(got, "<skill name=\"nginx-triage\">") {
		t.Errorf("expected XML-style skill tag, got %q", got)
	}
	if !strings.Contains(got, "Focus on access.log") {
		t.Errorf("expected skill prompt body, got %q", got)
	}
	if !strings.Contains(got, bare) {
		t.Errorf("expected user input preserved, got %q", got)
	}
	// Cache stability: same inputs MUST produce byte-identical output.
	if WrapUserInput(bare, skill) != got {
		t.Errorf("WrapUserInput is not deterministic")
	}
}

// routerStubProvider returns a fixed Response for any Complete call.
type routerStubProvider struct {
	resp *llm.Response
	err  error
	gotSystem string
}

func (s *routerStubProvider) Name() string { return "stub" }
func (s *routerStubProvider) Complete(_ context.Context, req llm.Request) (*llm.Response, error) {
	s.gotSystem = req.System
	if s.err != nil {
		return nil, s.err
	}
	return s.resp, nil
}

func TestLLMRouter_RouteHappyPath(t *testing.T) {
	p := &routerStubProvider{
		resp: &llm.Response{
			Message: llm.Message{Role: llm.RoleAssistant, Content: `{"skill":"nginx-triage"}`},
		},
	}
	r := &LLMRouter{Provider: p, Model: "m"}
	skills := []*Skill{
		{Name: "nginx-triage", Description: "triage nginx 5xx"},
		{Name: "kernel-probe", Description: "kernel info"},
	}
	got, err := r.Route(context.Background(), "502 errors everywhere", skills)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if got != "nginx-triage" {
		t.Errorf("picked %q want nginx-triage", got)
	}
	// Cache stability check: system prompt must contain BOTH skills, not the user input.
	if !strings.Contains(p.gotSystem, "nginx-triage") || !strings.Contains(p.gotSystem, "kernel-probe") {
		t.Errorf("router system prompt missing skills: %q", p.gotSystem)
	}
	if strings.Contains(p.gotSystem, "502 errors") {
		t.Errorf("user input leaked into system (would break cache stability): %q", p.gotSystem)
	}
}

func TestLLMRouter_RejectsHallucinatedSkill(t *testing.T) {
	p := &routerStubProvider{
		resp: &llm.Response{
			Message: llm.Message{Role: llm.RoleAssistant, Content: `{"skill":"nonexistent"}`},
		},
	}
	r := &LLMRouter{Provider: p, Model: "m"}
	got, err := r.Route(context.Background(), "do something",
		[]*Skill{{Name: "real", Description: "actual skill"}})
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty (hallucinated skill rejected), got %q", got)
	}
}

func TestLLMRouter_NullSkill(t *testing.T) {
	p := &routerStubProvider{
		resp: &llm.Response{Message: llm.Message{Content: `{"skill": null}`}},
	}
	r := &LLMRouter{Provider: p, Model: "m"}
	got, err := r.Route(context.Background(), "small talk", []*Skill{{Name: "x", Description: "y"}})
	if err != nil || got != "" {
		t.Errorf("expected empty on null, got (%q, %v)", got, err)
	}
}

func TestLLMRouter_EmptyCandidates(t *testing.T) {
	p := &routerStubProvider{}
	r := &LLMRouter{Provider: p, Model: "m"}
	got, err := r.Route(context.Background(), "anything", nil)
	if err != nil || got != "" {
		t.Errorf("expected empty when no skills, got (%q, %v)", got, err)
	}
	if p.gotSystem != "" {
		t.Errorf("provider should not have been called when no candidates")
	}
}

func TestLLMRouter_ProviderError(t *testing.T) {
	p := &routerStubProvider{err: errors.New("network down")}
	r := &LLMRouter{Provider: p, Model: "m"}
	_, err := r.Route(context.Background(), "x", []*Skill{{Name: "y", Description: "z"}})
	if err == nil {
		t.Errorf("expected error to propagate")
	}
}

func TestBuildRouterSystem_DeterministicForCacheStability(t *testing.T) {
	skills := []*Skill{
		{Name: "a", Description: "first"},
		{Name: "b", Description: "second"},
	}
	s1 := buildRouterSystem(skills)
	s2 := buildRouterSystem(skills)
	if s1 != s2 {
		t.Errorf("router system prompt is not deterministic — would burn prefix cache on every call")
	}
}
