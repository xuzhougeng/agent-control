package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"cc-agent/internal/llm"
)

// Router decides which skill (if any) should be applied to a fresh user turn.
// The agent calls Route before persisting the user message; when a skill is
// picked, its prompt is woven into the user message itself by WrapUserInput
// (NOT into req.System, which would invalidate the provider's prefix cache).
//
// An empty returned name means "no skill applies" — the agent runs with the
// raw user input.
type Router interface {
	Route(ctx context.Context, userInput string, available []*Skill) (skillName string, err error)
}

// LLMRouter delegates the routing decision to an LLM. Cache stability:
// req.System is built from the candidate skill list, so as long as the skill
// set is unchanged the system prompt is byte-identical across all router
// calls — DeepSeek-style prefix caches cover most of the cost. Only the
// per-turn user input is fresh.
//
// The router is cheap on purpose: small MaxTokens, optional separate model.
type LLMRouter struct {
	Provider llm.Provider
	Model    string
}

func (r *LLMRouter) Route(ctx context.Context, userInput string, available []*Skill) (string, error) {
	if r.Provider == nil {
		return "", errors.New("router: provider not configured")
	}
	if len(available) == 0 {
		return "", nil
	}
	system := buildRouterSystem(available)
	resp, err := r.Provider.Complete(ctx, llm.Request{
		System:    system,
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: userInput}},
		Model:     r.Model,
		MaxTokens: 100,
	})
	if err != nil {
		return "", err
	}
	picked := parseRouteDecision(resp.Message.Content)
	if picked == "" {
		return "", nil
	}
	// Defensive: only return a name we actually showed the model.
	for _, s := range available {
		if s.Name == picked {
			return picked, nil
		}
	}
	return "", nil
}

const routerSystemHeader = `You are a skill router. From the candidate skills below, pick THE ONE skill that most directly applies to the user's request. If no skill clearly applies, pick none.

Output rules:
- Output a single JSON object on one line. NO markdown fences, NO commentary.
- Use exactly one of these shapes:
    {"skill": "skill-name"}
    {"skill": null}
- The skill-name MUST be one of the candidates listed below — never invent one.

Candidate skills:
`

// buildRouterSystem produces a deterministic system prompt: the same skill
// set always yields byte-identical output. That stability is what keeps the
// router's prefix cache warm. Skill order follows the order passed in, so
// callers should pass a sorted slice for full stability across processes.
func buildRouterSystem(skills []*Skill) string {
	var b strings.Builder
	b.WriteString(routerSystemHeader)
	for _, s := range skills {
		fmt.Fprintf(&b, "- %s: %s\n", s.Name, s.Description)
	}
	return b.String()
}

var jsonFenceRouteRE = regexp.MustCompile("(?s)```(?:json)?\\s*(.+?)\\s*```")

// parseRouteDecision tolerates the usual model output drift: surrounding
// prose, markdown fences, trailing commentary. Returns the picked skill name
// or "" for "no skill" / parse failure.
func parseRouteDecision(raw string) string {
	raw = strings.TrimSpace(raw)
	if m := jsonFenceRouteRE.FindStringSubmatch(raw); m != nil {
		raw = m[1]
	}
	if i := strings.Index(raw, "{"); i > 0 {
		raw = raw[i:]
	}
	if j := strings.LastIndex(raw, "}"); j >= 0 && j < len(raw)-1 {
		raw = raw[:j+1]
	}
	var d struct {
		Skill *string `json:"skill"`
	}
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return ""
	}
	if d.Skill == nil {
		return ""
	}
	return strings.TrimSpace(*d.Skill)
}

// WrapUserInput weaves a skill's prompt into the user message. The wrapped
// form is what gets persisted to memory and replayed verbatim every
// subsequent turn — that immutability is what keeps the LLM's prefix cache
// warm across the session even when later turns route to different skills.
//
// Returns the input unchanged when skill is nil or has no prompt to add.
func WrapUserInput(userInput string, skill *Skill) string {
	if skill == nil || strings.TrimSpace(skill.Prompt) == "" {
		return userInput
	}
	return fmt.Sprintf("<skill name=%q>\n%s\n</skill>\n\n%s", skill.Name, skill.Prompt, userInput)
}
