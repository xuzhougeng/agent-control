package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"cc-agent/internal/llm"
)

// TranscriptItem is one normalized turn in a session, ready to feed to the
// reflector. The Reflector package can't import memory/llm without cycling
// the agent package, so we accept this small adapter type.
type TranscriptItem struct {
	Role     string // "user" | "assistant" | "tool"
	Text     string // assistant prose, user prompt, or tool result snippet
	ToolName string // when assistant emitted a tool_use
	ToolArgs string // short arg summary for tool_use
}

// DistillInput is what the agent hands to a Reflector. Name/Description are
// the operator's hints; the reflector may polish them.
type DistillInput struct {
	Name        string
	Description string
	History     []TranscriptItem
}

// Reflector turns a session transcript into a reusable Skill. Implementations
// usually delegate to an LLM, but a simpler heuristic-based reflector could
// be substituted for tests or offline use.
type Reflector interface {
	Distill(ctx context.Context, in DistillInput) (*Skill, error)
}

// LLMReflector uses the configured provider to write a skill JSON given the
// session transcript. The prompt instructs the model to emit a single JSON
// object describing the task pattern, useful guidance, and example requests.
type LLMReflector struct {
	Provider llm.Provider
	Model    string
}

func (r *LLMReflector) Distill(ctx context.Context, in DistillInput) (*Skill, error) {
	if r.Provider == nil {
		return nil, errors.New("reflect: provider not configured")
	}
	if len(in.History) == 0 {
		return nil, errors.New("reflect: empty history; nothing to distill")
	}
	tools := uniqueToolNames(in.History)
	prompt := buildReflectPrompt(in, tools)

	resp, err := r.Provider.Complete(ctx, llm.Request{
		System:    reflectSystemPrompt,
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: prompt}},
		Model:     r.Model,
		MaxTokens: 1500,
	})
	if err != nil {
		return nil, fmt.Errorf("reflect: llm complete: %w", err)
	}
	skill, err := parseSkillJSON(resp.Message.Content)
	if err != nil {
		return nil, fmt.Errorf("reflect: parse: %w (raw: %q)", err, resp.Message.Content)
	}
	if in.Name != "" {
		skill.Name = in.Name
	}
	if in.Description != "" {
		skill.Description = in.Description
	}
	skill.Name = sanitizeName(skill.Name)
	if skill.Name == "" {
		return nil, errors.New("reflect: distilled skill has no name; pass one explicitly")
	}
	if len(skill.Tools) == 0 {
		skill.Tools = tools
	}
	return skill, nil
}

const reflectSystemPrompt = `You are a skill distillation assistant. Given a transcript of a task that an LLM agent solved, produce a reusable skill definition.

Output ONLY a single JSON object with these fields, nothing else:
{
  "name": "kebab-case identifier, max 40 chars",
  "description": "one sentence describing what this skill helps with",
  "prompt": "a short system-prompt fragment (2-6 sentences) priming the agent for similar tasks. Include the high-leverage tactics from the transcript: which tool to try first, what to inspect, common pitfalls. Do not paste the full transcript.",
  "examples": ["example user request 1", "example user request 2"]
}

Rules:
- Be specific about file paths, command flags, and decision criteria observed in the transcript when they generalize.
- Do not invent capabilities the agent does not have.
- Keep "prompt" tight (under ~120 words). It is appended to the agent's system prompt at runtime.
- Output strict JSON. No markdown fences, no commentary.`

func buildReflectPrompt(in DistillInput, tools []string) string {
	var b strings.Builder
	if in.Name != "" {
		fmt.Fprintf(&b, "Operator-provided skill name: %q\n", in.Name)
	}
	if in.Description != "" {
		fmt.Fprintf(&b, "Operator-provided description: %q\n", in.Description)
	}
	if len(tools) > 0 {
		fmt.Fprintf(&b, "Tools observed in transcript: %s\n", strings.Join(tools, ", "))
	}
	b.WriteString("\nTranscript (oldest first; tool results truncated):\n")
	for i, it := range in.History {
		switch it.Role {
		case "user":
			fmt.Fprintf(&b, "[%d] USER: %s\n", i, clipForReflect(it.Text, 800))
		case "assistant":
			if it.ToolName != "" {
				fmt.Fprintf(&b, "[%d] ASSISTANT tool_use: %s %s\n", i, it.ToolName, it.ToolArgs)
			}
			if it.Text != "" {
				fmt.Fprintf(&b, "[%d] ASSISTANT: %s\n", i, clipForReflect(it.Text, 600))
			}
		case "tool":
			fmt.Fprintf(&b, "[%d] TOOL_RESULT: %s\n", i, clipForReflect(it.Text, 400))
		}
	}
	b.WriteString("\nProduce the JSON skill now.")
	return b.String()
}

func clipForReflect(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func uniqueToolNames(items []TranscriptItem) []string {
	seen := map[string]struct{}{}
	for _, it := range items {
		if it.ToolName != "" {
			seen[it.ToolName] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

var jsonFenceRE = regexp.MustCompile("(?s)```(?:json)?\\s*(.+?)\\s*```")

func parseSkillJSON(raw string) (*Skill, error) {
	raw = strings.TrimSpace(raw)
	if m := jsonFenceRE.FindStringSubmatch(raw); m != nil {
		raw = m[1]
	}
	// Some models prefix with stray prose; cut to the first '{'.
	if i := strings.Index(raw, "{"); i > 0 {
		raw = raw[i:]
	}
	if j := strings.LastIndex(raw, "}"); j >= 0 && j < len(raw)-1 {
		raw = raw[:j+1]
	}
	var s Skill
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return nil, err
	}
	return &s, nil
}

var unsafeNameRE = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ToLower(name)
	name = unsafeNameRE.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if len(name) > 64 {
		name = name[:64]
	}
	return name
}

// Save writes a skill to <dir>/<name>.json with 0644 perms and reloads the
// registry entry so subsequent Get returns the fresh definition. dir is
// created if missing.
func (r *Registry) Save(skill *Skill, dir string) (string, error) {
	if dir == "" {
		return "", errors.New("save: skills_dir is empty")
	}
	if skill.Name == "" {
		return "", errors.New("save: skill has no name")
	}
	if err := mkdirAll(dir); err != nil {
		return "", err
	}
	path := filepath.Join(dir, skill.Name+".json")
	raw, err := json.MarshalIndent(skill, "", "  ")
	if err != nil {
		return "", err
	}
	if err := writeFile(path, raw); err != nil {
		return "", err
	}
	skill.SourcePath = path
	r.skills[skill.Name] = skill
	return path, nil
}
