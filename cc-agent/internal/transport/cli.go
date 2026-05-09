// Package transport hosts the user-facing entry points: a local CLI REPL, an
// optional HTTP API, and a future cc-control WS bridge.
package transport

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	prompt "github.com/c-bata/go-prompt"

	"cc-agent/internal/agent"
	"cc-agent/internal/skills"
)

// CLIApprover prompts the operator on stderr/stdin for destructive commands.
// While inside go-prompt's Executor the terminal is in cooked mode, so a fresh
// bufio.Reader on stdin is enough — no fight with the prompt loop.
type CLIApprover struct {
	Reader *bufio.Reader
	Writer io.Writer
}

func NewCLIApprover(r *bufio.Reader, w io.Writer) *CLIApprover {
	if r == nil {
		r = bufio.NewReader(os.Stdin)
	}
	return &CLIApprover{Reader: r, Writer: w}
}

func (a *CLIApprover) Approve(_ context.Context, cmd, reason string) (bool, error) {
	fmt.Fprintf(a.Writer, "\n\033[31m⚠ destructive command\033[0m  reason: %s\n  command: %s\n  approve? [y/N]: ", reason, cmd)
	line, err := a.Reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	ans := strings.ToLower(strings.TrimSpace(line))
	return ans == "y" || ans == "yes", nil
}

// slashCmds is the list shown in the autocomplete dropdown when the user types `:`.
var slashCmds = []prompt.Suggest{
	{Text: ":help", Description: "show commands"},
	{Text: ":tools", Description: "list registered tools"},
	{Text: ":skills", Description: "list loaded skills"},
	{Text: ":reflect", Description: "distill current session into a skill"},
	{Text: ":registry", Description: "list team-registry skills"},
	{Text: ":publish", Description: "push a local skill to the team registry"},
	{Text: ":install", Description: "fetch + install a team skill"},
	{Text: ":history", Description: "show all versions of a team skill"},
	{Text: ":rollback", Description: "install a specific older version"},
	{Text: "exit", Description: "leave the REPL"},
	{Text: "quit", Description: "leave the REPL"},
}

// registryCache lazily memoizes the team-registry skill listing so the
// completer doesn't hit HTTP on every keystroke.
type registryCache struct {
	once sync.Once
	rows []prompt.Suggest
}

func (c *registryCache) Names(rc *skills.RegistryClient) []prompt.Suggest {
	if rc == nil {
		return nil
	}
	c.once.Do(func() {
		rows, err := rc.List("")
		if err != nil {
			return
		}
		for _, s := range rows {
			c.rows = append(c.rows, prompt.Suggest{Text: s.Name, Description: s.Description})
		}
	})
	return c.rows
}

// makeCompleter returns a prompt.Completer that suggests slash commands and
// then context-sensitive arguments based on which sub-command is being typed.
//
// Behavior summary:
//
//	(empty)             → no suggestions (typing prose is the common case)
//	:                   → all slash commands
//	:install <prefix>   → registry skill names
//	:rollback <prefix>  → registry skill names (version arg is free-form)
//	:history  <prefix>  → registry skill names
//	:publish  <prefix>  → local skill names
//	:reflect  <prefix>  → local skill names (existing names → overwrite)
func makeCompleter(ag *agent.Agent, rc *skills.RegistryClient) prompt.Completer {
	cache := &registryCache{}
	localNames := func() []prompt.Suggest {
		reg := ag.Skills()
		if reg == nil {
			return nil
		}
		var out []prompt.Suggest
		for _, n := range reg.Names() {
			s, _ := reg.Get(n)
			out = append(out, prompt.Suggest{Text: s.Name, Description: s.Description})
		}
		return out
	}
	return func(d prompt.Document) []prompt.Suggest {
		text := d.TextBeforeCursor()
		trimmed := strings.TrimLeft(text, " \t")
		if trimmed == "" {
			return nil
		}
		// First word: still typing the command itself.
		if !strings.ContainsAny(trimmed, " \t") {
			if !strings.HasPrefix(trimmed, ":") && !strings.HasPrefix(trimmed, "e") && !strings.HasPrefix(trimmed, "q") {
				return nil
			}
			return prompt.FilterHasPrefix(slashCmds, trimmed, true)
		}
		// Past the command — pick suggestions based on the verb.
		fields := strings.Fields(trimmed)
		word := d.GetWordBeforeCursor()
		switch fields[0] {
		case ":install", ":rollback", ":history":
			// Only the first arg is a skill name.
			if argIndex(trimmed, fields) > 1 {
				return nil
			}
			return prompt.FilterHasPrefix(cache.Names(rc), word, true)
		case ":publish", ":reflect":
			if argIndex(trimmed, fields) > 1 {
				return nil
			}
			return prompt.FilterHasPrefix(localNames(), word, true)
		}
		return nil
	}
}

// argIndex returns 0 if the cursor is on the verb, 1 if on the first arg, etc.
// Uses trailing-space detection because strings.Fields collapses whitespace.
func argIndex(text string, fields []string) int {
	idx := len(fields) - 1
	if strings.HasSuffix(text, " ") || strings.HasSuffix(text, "\t") {
		idx++
	}
	return idx
}

// RunCLI runs the stdin REPL with autocomplete dropdown (arrow-key selectable),
// persistent history, and Ctrl+C semantics scoped to the current turn.
//
// approver is optional: when non-nil it gets a bufio.Reader bound to stdin so
// destructive-command prompts work cleanly inside the Executor (terminal is
// cooked while the executor runs).
func RunCLI(ctx context.Context, ag *agent.Agent, rc *skills.RegistryClient, sessionID string, approver *CLIApprover) error {
	if approver != nil && approver.Reader == nil {
		approver.Reader = bufio.NewReader(os.Stdin)
	}

	historyPath := historyFilePath()
	history := loadHistory(historyPath, 1000)

	ag.SetListener(func(e agent.Event) {
		switch e.Kind {
		case agent.EventAssistant:
			fmt.Printf("\n\033[36massistant>\033[0m %s\n", e.Text)
		case agent.EventToolCall:
			fmt.Printf("\033[33mtool>\033[0m %s %s\n", e.ToolName, summarizeInput(e.ToolInput))
		case agent.EventToolResult:
			fmt.Printf("\033[90m%s\033[0m\n", clip(e.Text, 800))
		case agent.EventError:
			fmt.Fprintf(os.Stderr, "\033[31merror>\033[0m %s\n", e.Text)
		}
	})

	fmt.Printf("cc-agent ready. session=%s. type ':' for commands (↑/↓ to pick), 'exit' or Ctrl+D to quit.\n", sessionID)

	executor := func(line string) {
		line = strings.TrimSpace(line)
		if line != "" {
			appendHistory(historyPath, line)
		}
		if line == "" {
			return
		}
		if line == "exit" || line == "quit" {
			os.Exit(0)
		}
		if strings.HasPrefix(line, ":") {
			if err := handleSlashCommand(ctx, ag, rc, sessionID, line, approver); err != nil {
				fmt.Fprintf(os.Stderr, "command error: %v\n", err)
			}
			return
		}
		runCtx, cancelRun := context.WithCancel(ctx)
		stop := installRunInterrupt(cancelRun)
		_, runErr := ag.Run(runCtx, sessionID, line)
		stop()
		cancelRun()
		switch {
		case runErr == nil:
		case errors.Is(runErr, context.Canceled):
			fmt.Fprintln(os.Stderr, "\n\033[33m^C interrupted current turn\033[0m")
		default:
			fmt.Fprintf(os.Stderr, "run error: %v\n", runErr)
		}
	}

	completer := makeCompleter(ag, rc)

	p := prompt.New(
		executor,
		completer,
		prompt.OptionPrefix("you> "),
		prompt.OptionTitle("cc-agent"),
		prompt.OptionHistory(history),
		prompt.OptionInputTextColor(prompt.DefaultColor),
		prompt.OptionSuggestionBGColor(prompt.DarkGray),
		prompt.OptionSuggestionTextColor(prompt.White),
		prompt.OptionDescriptionBGColor(prompt.DarkGray),
		prompt.OptionDescriptionTextColor(prompt.LightGray),
		prompt.OptionSelectedSuggestionBGColor(prompt.Cyan),
		prompt.OptionSelectedSuggestionTextColor(prompt.Black),
		prompt.OptionSelectedDescriptionBGColor(prompt.Cyan),
		prompt.OptionSelectedDescriptionTextColor(prompt.Black),
		prompt.OptionAddKeyBind(prompt.KeyBind{
			Key: prompt.ControlD,
			Fn:  func(buf *prompt.Buffer) { fmt.Println("bye."); os.Exit(0) },
		}),
		prompt.OptionSetExitCheckerOnInput(func(in string, breakline bool) bool {
			s := strings.TrimSpace(in)
			return breakline && (s == "exit" || s == "quit")
		}),
	)
	p.Run()
	return nil
}

// installRunInterrupt makes Ctrl+C cancel the current agent turn instead of
// the whole process. The returned stop() must be called once the turn ends so
// subsequent SIGINTs flow back to go-prompt's input handler at the next prompt.
func installRunInterrupt(cancel context.CancelFunc) func() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT)
	go func() {
		for range ch {
			cancel()
		}
	}()
	return func() {
		signal.Stop(ch)
		close(ch)
	}
}

func historyFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".cc-agent")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "history")
}

func loadHistory(path string, max int) []string {
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		t := strings.TrimRight(sc.Text(), "\r\n")
		if t == "" {
			continue
		}
		lines = append(lines, t)
	}
	if len(lines) > max {
		lines = lines[len(lines)-max:]
	}
	return lines
}

func appendHistory(path, line string) {
	if path == "" || line == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line + "\n")
}

// handleSlashCommand dispatches REPL meta commands. Currently:
//
//	:help                         show commands
//	:skills                       list loaded skills
//	:reflect <name> [description] distill the current session into a skill
//	:registry / :publish / :install / :history / :rollback   team registry ops
func handleSlashCommand(ctx context.Context, ag *agent.Agent, rc *skills.RegistryClient, sessionID, line string, approver *CLIApprover) error {
	parts := strings.Fields(line)
	cmd := parts[0]
	switch cmd {
	case ":help", ":?":
		fmt.Println(`commands:
  :help                            show this help
  :tools                           list registered tools
  :skills                          list loaded skills
  :reflect <name> [description]    distill current session into a skill
  :registry [search]               list team skills
  :publish <name>                  push a local skill to the team registry
  :install <name>[@version]        fetch + install a team skill (with preview)
  :history <name>                  show all versions of a team skill
  :rollback <name> <version>       install a specific older version
  exit | quit | Ctrl+D             leave (Ctrl+C cancels current turn)`)
		return nil
	case ":tools":
		reg := ag.Tools()
		if reg == nil {
			fmt.Println("(no tool registry)")
			return nil
		}
		all := reg.All()
		if len(all) == 0 {
			fmt.Println("(no tools registered)")
			return nil
		}
		for _, t := range all {
			fmt.Printf("  - %-20s %s\n", t.Name(), clip(t.Description(), 90))
		}
		return nil
	case ":skills":
		reg := ag.Skills()
		if reg == nil {
			fmt.Println("(skills registry not configured)")
			return nil
		}
		names := reg.Names()
		if len(names) == 0 {
			fmt.Println("(no skills loaded)")
			return nil
		}
		for _, n := range names {
			s, _ := reg.Get(n)
			fmt.Printf("  - %-30s %s\n", s.Name, s.Description)
		}
		return nil
	case ":reflect":
		if len(parts) < 2 {
			return fmt.Errorf("usage: :reflect <name> [description]")
		}
		name := parts[1]
		desc := ""
		if len(parts) > 2 {
			desc = strings.Join(parts[2:], " ")
		}
		fmt.Println("\033[33mdistilling session into skill...\033[0m")
		skill, path, err := ag.Reflect(ctx, sessionID, name, desc)
		if err != nil {
			return err
		}
		fmt.Printf("\033[32m✓ saved skill\033[0m %s -> %s\n", skill.Name, path)
		fmt.Printf("  description: %s\n", skill.Description)
		fmt.Printf("  tools: %s\n", strings.Join(skill.Tools, ", "))
		if len(skill.Examples) > 0 {
			fmt.Println("  examples:")
			for _, e := range skill.Examples {
				fmt.Printf("    - %s\n", e)
			}
		}
		return nil
	case ":registry":
		if rc == nil {
			fmt.Println("(registry not configured: set control_http_url + agent_token)")
			return nil
		}
		q := ""
		if len(parts) > 1 {
			q = strings.Join(parts[1:], " ")
		}
		rows, err := rc.List(q)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			fmt.Println("(no skills in registry)")
			return nil
		}
		for _, s := range rows {
			fmt.Printf("  %-30s v%-3d %-12s %s\n", s.Name, s.Version, s.AuthorServerID, s.Description)
		}
		return nil
	case ":publish":
		if rc == nil {
			fmt.Println("(registry not configured)")
			return nil
		}
		if len(parts) < 2 {
			return fmt.Errorf("usage: :publish <name>")
		}
		name := parts[1]
		reg := ag.Skills()
		if reg == nil {
			return fmt.Errorf("no local skills registry")
		}
		sk, ok := reg.Get(name)
		if !ok {
			return fmt.Errorf("no local skill %q (try :skills to list)", name)
		}
		wire := &skills.Skill{
			Name:        sk.Name,
			Description: sk.Description,
			Prompt:      sk.Prompt,
			Tools:       sk.Tools,
			Examples:    sk.Examples,
		}
		v, err := rc.Publish(wire)
		if err != nil {
			return err
		}
		fmt.Printf("\033[32m✓ published\033[0m %s@%d\n", name, v)
		return nil
	case ":install":
		if rc == nil {
			fmt.Println("(registry not configured)")
			return nil
		}
		if len(parts) < 2 {
			return fmt.Errorf("usage: :install <name>[@version]")
		}
		name, version := parseNameVersion(parts[1])
		// Fetch without writing first to render the preview.
		preview := *rc       // shallow copy
		preview.TeamDir = "" // disables file write
		got, err := preview.Install(name, version)
		if err != nil {
			return err
		}
		fmt.Println("\033[36m── skill preview ──\033[0m")
		fmt.Printf("  name:    %s @ v%d\n", got.Name, got.Version)
		fmt.Printf("  author:  %s\n", got.AuthorServerID)
		fmt.Printf("  updated: %s\n", time.Unix(got.CreatedAtUnix, 0).Format(time.RFC3339))
		fmt.Printf("  tools:   %s\n", strings.Join(got.Tools, ", "))
		fmt.Printf("  prompt:  %s\n", clip(got.Prompt, 200))
		if len(got.Examples) > 0 {
			fmt.Println("  examples:")
			for _, e := range got.Examples {
				fmt.Printf("    - %s\n", clip(e, 80))
			}
		}
		fmt.Print("install? [y/N]: ")
		ans, err := readApproverLine(approver)
		if err != nil {
			return err
		}
		ans = strings.TrimSpace(strings.ToLower(ans))
		if ans != "y" && ans != "yes" {
			fmt.Println("aborted.")
			return nil
		}
		if _, err := rc.Install(name, version); err != nil {
			return err
		}
		teamDir := rc.TeamDir
		fmt.Printf("\033[32m✓ installed\033[0m %s@%d → %s/%s.json\n", got.Name, got.Version, teamDir, got.Name)
		if reg := ag.Skills(); reg != nil {
			_ = reg.LoadDir(teamDir)
		}
		return nil
	case ":history":
		if rc == nil {
			fmt.Println("(registry not configured)")
			return nil
		}
		if len(parts) < 2 {
			return fmt.Errorf("usage: :history <name>")
		}
		hist, err := rc.History(parts[1])
		if err != nil {
			return err
		}
		if len(hist) == 0 {
			fmt.Println("(no history)")
			return nil
		}
		for _, h := range hist {
			fmt.Printf("  v%-3d %-12s %s  %s\n", h.Version, h.AuthorServerID,
				time.Unix(h.CreatedAtUnix, 0).Format(time.RFC3339), h.Description)
		}
		return nil
	case ":rollback":
		if rc == nil {
			fmt.Println("(registry not configured)")
			return nil
		}
		if len(parts) < 3 {
			return fmt.Errorf("usage: :rollback <name> <version>")
		}
		v, err := strconv.Atoi(parts[2])
		if err != nil {
			return err
		}
		if _, err := rc.Install(parts[1], v); err != nil {
			return err
		}
		fmt.Printf("\033[32m✓ rolled back\033[0m %s to v%d\n", parts[1], v)
		if reg := ag.Skills(); reg != nil {
			_ = reg.LoadDir(rc.TeamDir)
		}
		return nil
	default:
		return fmt.Errorf("unknown command %q (try :help)", cmd)
	}
}

// readApproverLine reads one stdin line using the approver's bufio.Reader if
// one exists, otherwise falls back to a fresh reader on os.Stdin. Used by the
// :install y/N prompt so it shares a buffer with the destructive-command prompt
// (avoids two readers fighting over half-buffered input).
func readApproverLine(a *CLIApprover) (string, error) {
	if a != nil && a.Reader != nil {
		return a.Reader.ReadString('\n')
	}
	return bufio.NewReader(os.Stdin).ReadString('\n')
}

func summarizeInput(in map[string]any) string {
	if len(in) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(in))
	for k, v := range in {
		s := fmt.Sprintf("%v", v)
		parts = append(parts, fmt.Sprintf("%s=%s", k, clip(s, 80)))
	}
	return "{" + strings.Join(parts, " ") + "}"
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(+" + fmt.Sprint(len(s)-n) + ")"
}

func parseNameVersion(s string) (string, int) {
	if i := strings.LastIndex(s, "@"); i > 0 {
		v, err := strconv.Atoi(s[i+1:])
		if err == nil {
			return s[:i], v
		}
	}
	return s, 0
}
