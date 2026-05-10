// cc-agent-eval scores the LLM router on a fixed dataset of prompts whose
// "right" skill is known by hand. Use it to detect router regressions when
// changing the router prompt, the candidate skill descriptions, or the
// model — and to compare providers/models head-to-head.
//
// Reads the API key from ~/.cc-agent-key by default (so secrets never end
// up in shell history). Provider / model / base URL are flag-controlled and
// match cc-agent's defaults.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"cc-agent/internal/llm"
	"cc-agent/internal/skills"
)

type Case struct {
	Prompt   string   `json:"prompt"`
	Expected *string  `json:"expected"` // pointer: distinguishes "" from null
	Tags     []string `json:"tags"`
	Notes    string   `json:"notes"`
}

type CaseResult struct {
	Case     Case
	Picks    []string      // one per run
	Hits     int           // runs where pick matched expected
	Latency  time.Duration // sum across runs
	Errors   []string
}

type Report struct {
	Provider     string                       `json:"provider"`
	Model        string                       `json:"model"`
	Cases        int                          `json:"cases"`
	Runs         int                          `json:"runs"`
	Hits         int                          `json:"hits"`
	Total        int                          `json:"total"`
	Accuracy     float64                      `json:"accuracy"`
	WallSeconds  float64                      `json:"wall_seconds"`
	PerSkill     map[string]map[string]int    `json:"per_skill"` // skill -> {hits, total}
	Cases_       []caseReportRow              `json:"cases_detail"`
}

type caseReportRow struct {
	Prompt   string   `json:"prompt"`
	Expected *string  `json:"expected"`
	Picks    []string `json:"picks"`
	Hits     int      `json:"hits"`
	Runs     int      `json:"runs"`
	AvgMs    int64    `json:"avg_ms"`
}

func main() {
	var (
		keyFile  = flag.String("key-file", defaultKeyFile(), "path to a file containing the API key (single line)")
		provider = flag.String("provider", envOr("CC_AGENT_PROVIDER", "anthropic"), "provider: anthropic | openai | deepseek | qwen | openai-compat")
		model    = flag.String("model", envOr("CC_AGENT_MODEL", ""), "model name (default: provider's default)")
		baseURL  = flag.String("base-url", envOr("CC_AGENT_BASE_URL", ""), "OpenAI-compat base URL (e.g. https://api.deepseek.com/v1)")
		skillDir = flag.String("skills-dir", "tests/eval/skills", "directory of skill JSON files")
		casesIn  = flag.String("cases", "tests/eval/cases.json", "JSON file of test cases")
		runs     = flag.Int("runs", 1, "how many times to run each case (LLM is non-deterministic)")
		timeout  = flag.Duration("timeout", 60*time.Second, "per-call timeout")
		jsonOut  = flag.Bool("json", false, "emit a single JSON report on stdout instead of human output")
		verbose  = flag.Bool("verbose", false, "print each call's result as it lands")
	)
	flag.Parse()

	apiKey := strings.TrimSpace(envOr("CC_AGENT_API_KEY", ""))
	if apiKey == "" && *keyFile != "" {
		raw, err := os.ReadFile(*keyFile)
		if err == nil {
			apiKey = strings.TrimSpace(string(raw))
		}
	}
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	}
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	if apiKey == "" {
		fatalf("no API key found: set -key-file, CC_AGENT_API_KEY, ANTHROPIC_API_KEY, or OPENAI_API_KEY")
	}

	if *model == "" {
		switch *provider {
		case "anthropic":
			*model = "claude-sonnet-4-6"
		case "deepseek":
			*model = "deepseek-chat"
		case "openai", "openai-compat":
			*model = "gpt-4o-mini"
		}
	}

	prov, err := llm.New(llm.Config{
		Provider: *provider,
		APIKey:   apiKey,
		BaseURL:  *baseURL,
		Timeout:  *timeout,
	})
	if err != nil {
		fatalf("provider: %v", err)
	}

	reg := skills.NewRegistry()
	if err := reg.LoadDir(*skillDir); err != nil {
		fatalf("load skills %s: %v", *skillDir, err)
	}
	skillNames := reg.Names()
	if len(skillNames) == 0 {
		fatalf("no skills loaded from %s", *skillDir)
	}
	candidates := make([]*skills.Skill, 0, len(skillNames))
	for _, n := range skillNames {
		if s, ok := reg.Get(n); ok {
			candidates = append(candidates, s)
		}
	}

	cases, err := loadCases(*casesIn)
	if err != nil {
		fatalf("load cases %s: %v", *casesIn, err)
	}
	validateExpectations(cases, skillNames)

	router := &skills.LLMRouter{Provider: prov, Model: *model}

	if !*jsonOut {
		fmt.Fprintf(os.Stderr, "loaded %d skills · %d cases · runs/case=%d · provider=%s model=%s\n\n",
			len(candidates), len(cases), *runs, *provider, *model)
	}

	start := time.Now()
	results := make([]CaseResult, len(cases))
	for i, c := range cases {
		results[i].Case = c
		for r := 0; r < *runs; r++ {
			ctx, cancel := context.WithTimeout(context.Background(), *timeout)
			t0 := time.Now()
			pick, err := router.Route(ctx, c.Prompt, candidates)
			elapsed := time.Since(t0)
			cancel()
			results[i].Latency += elapsed
			if err != nil {
				results[i].Errors = append(results[i].Errors, err.Error())
				results[i].Picks = append(results[i].Picks, "")
			} else {
				results[i].Picks = append(results[i].Picks, pick)
				if matchExpected(c.Expected, pick) {
					results[i].Hits++
				}
			}
			if *verbose && !*jsonOut {
				printVerbose(i+1, len(cases), r+1, *runs, c, pick, err, elapsed)
			}
		}
		if !*verbose && !*jsonOut {
			printCaseLine(i+1, len(cases), results[i])
		}
	}
	wall := time.Since(start)

	if *jsonOut {
		emitJSON(os.Stdout, *provider, *model, *runs, results, wall)
		return
	}
	emitHuman(os.Stderr, *runs, results, wall)

	// exit 1 if any case missed all runs — useful for CI gating.
	for _, r := range results {
		if r.Hits == 0 {
			os.Exit(1)
		}
	}
}

func defaultKeyFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cc-agent-key")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func loadCases(path string) ([]Case, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []Case
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// validateExpectations refuses to start if a case names a skill the registry
// doesn't actually have — would otherwise look like a router miss every time.
func validateExpectations(cases []Case, skillNames []string) {
	known := map[string]bool{}
	for _, n := range skillNames {
		known[n] = true
	}
	for i, c := range cases {
		if c.Expected == nil {
			continue
		}
		if !known[*c.Expected] {
			fatalf("case %d expects skill %q which is not in the loaded set", i, *c.Expected)
		}
	}
}

func matchExpected(expected *string, pick string) bool {
	if expected == nil {
		return pick == ""
	}
	return *expected == pick
}

func printVerbose(caseIdx, caseTotal, runIdx, runTotal int, c Case, pick string, err error, elapsed time.Duration) {
	want := "<none>"
	if c.Expected != nil {
		want = *c.Expected
	}
	got := pick
	if got == "" {
		got = "<none>"
	}
	verdict := "PASS"
	if err != nil {
		verdict = fmt.Sprintf("ERR (%v)", err)
	} else if !matchExpected(c.Expected, pick) {
		verdict = fmt.Sprintf("FAIL want=%s got=%s", want, got)
	}
	fmt.Fprintf(os.Stderr, "[%d/%d r%d/%d] %s  (%dms)  prompt=%q\n",
		caseIdx, caseTotal, runIdx, runTotal, verdict, elapsed.Milliseconds(), clip(c.Prompt, 60))
}

func printCaseLine(caseIdx, caseTotal int, r CaseResult) {
	want := "<none>"
	if r.Case.Expected != nil {
		want = *r.Case.Expected
	}
	gotSummary := summarizePicks(r.Picks)
	verdict := fmt.Sprintf("%d/%d", r.Hits, len(r.Picks))
	tag := "PASS"
	switch {
	case r.Hits == 0 && len(r.Errors) == len(r.Picks):
		tag = "ERR"
	case r.Hits == 0:
		tag = "FAIL"
	case r.Hits < len(r.Picks):
		tag = "PART"
	}
	fmt.Fprintf(os.Stderr, "[%d/%d] %-4s %s  want=%-15s got=%s  prompt=%q\n",
		caseIdx, caseTotal, tag, verdict, want, gotSummary, clip(r.Case.Prompt, 60))
}

func summarizePicks(picks []string) string {
	if len(picks) == 1 {
		if picks[0] == "" {
			return "<none>"
		}
		return picks[0]
	}
	counts := map[string]int{}
	for _, p := range picks {
		key := p
		if key == "" {
			key = "<none>"
		}
		counts[key]++
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return counts[keys[i]] > counts[keys[j]] })
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s×%d", k, counts[k])
	}
	return b.String()
}

func emitHuman(w io.Writer, runs int, results []CaseResult, wall time.Duration) {
	totalCalls := 0
	totalHits := 0
	perSkill := map[string][2]int{} // skill or "<none>" -> [hits, total]
	for _, r := range results {
		totalCalls += len(r.Picks)
		totalHits += r.Hits
		key := "<none>"
		if r.Case.Expected != nil {
			key = *r.Case.Expected
		}
		ht := perSkill[key]
		ht[0] += r.Hits
		ht[1] += len(r.Picks)
		perSkill[key] = ht
	}
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "============================================================")
	fmt.Fprintf(w, "overall: %d/%d (%.1f%%) · wall %.1fs · runs/case=%d\n",
		totalHits, totalCalls, percent(totalHits, totalCalls), wall.Seconds(), runs)
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "per expected-skill:")
	keys := make([]string, 0, len(perSkill))
	for k := range perSkill {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		ht := perSkill[k]
		fmt.Fprintf(w, "  %-20s %d/%d (%.1f%%)\n", k, ht[0], ht[1], percent(ht[0], ht[1]))
	}
}

func emitJSON(w io.Writer, provider, model string, runs int, results []CaseResult, wall time.Duration) {
	rep := Report{
		Provider:    provider,
		Model:       model,
		Cases:       len(results),
		Runs:        runs,
		WallSeconds: wall.Seconds(),
		PerSkill:    map[string]map[string]int{},
	}
	for _, r := range results {
		rep.Hits += r.Hits
		rep.Total += len(r.Picks)
		key := "<none>"
		if r.Case.Expected != nil {
			key = *r.Case.Expected
		}
		ps := rep.PerSkill[key]
		if ps == nil {
			ps = map[string]int{"hits": 0, "total": 0}
		}
		ps["hits"] += r.Hits
		ps["total"] += len(r.Picks)
		rep.PerSkill[key] = ps
		row := caseReportRow{
			Prompt:   r.Case.Prompt,
			Expected: r.Case.Expected,
			Picks:    r.Picks,
			Hits:     r.Hits,
			Runs:     len(r.Picks),
		}
		if len(r.Picks) > 0 {
			row.AvgMs = r.Latency.Milliseconds() / int64(len(r.Picks))
		}
		rep.Cases_ = append(rep.Cases_, row)
	}
	rep.Accuracy = percent(rep.Hits, rep.Total) / 100
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(rep)
}

func percent(num, denom int) float64 {
	if denom == 0 {
		return 0
	}
	return float64(num) / float64(denom) * 100
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "cc-agent-eval: "+format+"\n", args...)
	os.Exit(2)
}
