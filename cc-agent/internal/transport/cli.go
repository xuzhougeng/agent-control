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

	"github.com/chzyer/readline"

	"cc-agent/internal/agent"
	"cc-agent/internal/skills"
)

// CLIApprover prompts the operator for destructive-command approval. The REPL
// is built on chzyer/readline whose IoLoop goroutine owns stdin while the
// instance is alive; reading via a fresh bufio.Reader would race the IoLoop
// and hang. So when wired in REPL mode we route reads through the readline
// instance itself. The Reader/Writer fields remain for tests and headless
// callers.
type CLIApprover struct {
	mu     sync.Mutex
	rl     *readline.Instance
	Reader *bufio.Reader
	Writer io.Writer
}

func NewCLIApprover(rl *readline.Instance, w io.Writer) *CLIApprover {
	a := &CLIApprover{rl: rl, Writer: w}
	if a.Writer == nil {
		if rl != nil {
			a.Writer = rl.Stderr()
		} else {
			a.Writer = os.Stderr
		}
	}
	return a
}

// SetReadline wires (or replaces) the readline instance after construction.
// main.go builds the approver before RunCLI has the instance, so this lets
// RunCLI plumb it in once readline is up.
func (a *CLIApprover) SetReadline(rl *readline.Instance) {
	a.mu.Lock()
	a.rl = rl
	if rl != nil {
		a.Writer = rl.Stderr()
	}
	a.mu.Unlock()
}

func (a *CLIApprover) Approve(_ context.Context, cmd, reason string) (bool, error) {
	fmt.Fprintf(a.Writer, "\n\033[31m⚠ destructive command\033[0m  reason: %s\n  command: %s\n", reason, cmd)
	line, err := a.AskLine("approve? [y/N]: ")
	if err != nil {
		if errors.Is(err, readline.ErrInterrupt) || errors.Is(err, io.EOF) {
			return false, nil
		}
		return false, err
	}
	ans := strings.ToLower(strings.TrimSpace(line))
	return ans == "y" || ans == "yes", nil
}

// AskLine prompts the operator for one line of input. Used both for
// destructive-command approval and the :install y/N confirm so all interactive
// reads share the readline pipeline.
func (a *CLIApprover) AskLine(prompt string) (string, error) {
	a.mu.Lock()
	rl := a.rl
	a.mu.Unlock()
	if rl != nil {
		prev := rl.Config.Prompt
		rl.SetPrompt(prompt)
		defer rl.SetPrompt(prev)
		return rl.Readline()
	}
	r := a.Reader
	if r == nil {
		r = bufio.NewReader(os.Stdin)
	}
	fmt.Fprint(a.Writer, prompt)
	return r.ReadString('\n')
}

// registryCache lazily memoizes the team-registry skill listing so the
// completer doesn't hit HTTP on every keystroke.
type registryCache struct {
	once sync.Once
	rows []string
}

func (c *registryCache) Names(rc *skills.RegistryClient) []string {
	if rc == nil {
		return nil
	}
	c.once.Do(func() {
		rows, err := rc.List("")
		if err != nil {
			return
		}
		for _, s := range rows {
			c.rows = append(c.rows, s.Name)
		}
	})
	return c.rows
}

// makeCompleter wires tab-completion: bare slash commands, plus dynamic
// arg-completion for sub-commands that take a skill name (local vs registry).
func makeCompleter(ag *agent.Agent, rc *skills.RegistryClient) readline.AutoCompleter {
	cache := &registryCache{}
	localNames := func(string) []string {
		reg := ag.Skills()
		if reg == nil {
			return nil
		}
		return append([]string(nil), reg.Names()...)
	}
	teamNames := func(string) []string {
		return append([]string(nil), cache.Names(rc)...)
	}
	return readline.NewPrefixCompleter(
		readline.PcItem(":help"),
		readline.PcItem(":tools"),
		readline.PcItem(":skills"),
		readline.PcItem(":reflect", readline.PcItemDynamic(localNames)),
		readline.PcItem(":registry"),
		readline.PcItem(":publish", readline.PcItemDynamic(localNames)),
		readline.PcItem(":install", readline.PcItemDynamic(teamNames)),
		readline.PcItem(":history", readline.PcItemDynamic(teamNames)),
		readline.PcItem(":rollback", readline.PcItemDynamic(teamNames)),
		readline.PcItem("exit"),
		readline.PcItem("quit"),
	)
}

// RunCLI runs the stdin REPL backed by chzyer/readline. Tab cycles through
// slash commands (and registry skill names where applicable), history
// persists to $HOME/.cc-agent/history, Ctrl+C at the prompt drops the
// partially-typed line, and Ctrl+D quits.
//
// approver, when non-nil, has its readline instance wired in so destructive-
// command prompts share the same input pipeline. A fresh bufio.Reader on
// os.Stdin would race the readline IoLoop goroutine and hang.
//
// routeVerbose prints "[router] picked skill: X" for each EventRouter so
// operators can see which skill the auto-router selected per turn.
func RunCLI(ctx context.Context, ag *agent.Agent, rc *skills.RegistryClient, sessionID string, approver *CLIApprover, routeVerbose bool) error {
	cfg := &readline.Config{
		Prompt:            "you> ",
		HistoryFile:       historyFilePath(),
		HistoryLimit:      1000,
		AutoComplete:      makeCompleter(ag, rc),
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		HistorySearchFold: true,
	}
	rl, err := readline.NewEx(cfg)
	if err != nil {
		return err
	}
	defer rl.Close()

	if approver != nil {
		approver.SetReadline(rl)
	}

	ag.SetListener(func(e agent.Event) {
		switch e.Kind {
		case agent.EventRouter:
			if routeVerbose {
				fmt.Fprintf(rl.Stdout(), "\033[35m[router]\033[0m picked skill: %s\n", e.Text)
			}
		case agent.EventAssistant:
			fmt.Fprintf(rl.Stdout(), "\n\033[36massistant>\033[0m %s\n", e.Text)
		case agent.EventToolCall:
			fmt.Fprintf(rl.Stdout(), "\033[33mtool>\033[0m %s %s\n", e.ToolName, summarizeInput(e.ToolInput))
		case agent.EventToolResult:
			fmt.Fprintf(rl.Stdout(), "\033[90m%s\033[0m\n", clip(e.Text, 800))
		case agent.EventError:
			fmt.Fprintf(rl.Stderr(), "\033[31merror>\033[0m %s\n", e.Text)
		}
	})

	fmt.Fprintf(rl.Stdout(), "cc-agent ready. session=%s. type ':' + Tab for commands, 'exit' or Ctrl+D to quit.\n", sessionID)

	for {
		line, err := rl.Readline()
		switch {
		case errors.Is(err, readline.ErrInterrupt):
			// Ctrl+C at the prompt: drop the partially-typed line, reprompt.
			continue
		case errors.Is(err, io.EOF):
			return nil
		case err != nil:
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" {
			return nil
		}
		if strings.HasPrefix(line, ":") {
			if err := handleSlashCommand(ctx, ag, rc, sessionID, line, approver); err != nil {
				fmt.Fprintf(rl.Stderr(), "command error: %v\n", err)
			}
			continue
		}
		runCtx, cancelRun := context.WithCancel(ctx)
		stop := installRunInterrupt(cancelRun)
		_, runErr := ag.Run(runCtx, sessionID, line)
		stop()
		cancelRun()
		switch {
		case runErr == nil:
		case errors.Is(runErr, context.Canceled):
			fmt.Fprintln(rl.Stderr(), "\n\033[33minterrupted current turn\033[0m")
		default:
			fmt.Fprintf(rl.Stderr(), "run error: %v\n", runErr)
		}
	}
}

// installRunInterrupt routes SIGINT (e.g. `kill -INT pid`) to the per-turn
// cancel during agent.Run. Note: while readline owns the terminal in raw mode
// the kernel does NOT generate SIGINT from Ctrl+C — that byte goes into the
// readline buffer instead. So this only fires when the user signals the
// process directly. When the loop returns, stop() is called so subsequent
// signals fall through to the default handler again.
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
  exit | quit | Ctrl+D             leave the REPL`)
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
		ans, err := readApproverLine(approver, "install? [y/N]: ")
		if err != nil {
			if errors.Is(err, readline.ErrInterrupt) || errors.Is(err, io.EOF) {
				fmt.Println("aborted.")
				return nil
			}
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

// readApproverLine asks the approver for one line. Falls back to a fresh
// stdin reader when no approver is wired (headless tests, daemon mode).
func readApproverLine(a *CLIApprover, prompt string) (string, error) {
	if a != nil {
		return a.AskLine(prompt)
	}
	fmt.Print(prompt)
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
