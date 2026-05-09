// Package transport hosts the user-facing entry points: a local CLI REPL, an
// optional HTTP API, and a future cc-control WS bridge.
package transport

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"cc-agent/internal/agent"
	"cc-agent/internal/skills"
)

// CLIApprover prompts the operator on stderr/stdin for destructive commands.
// Reads from the same Reader as the REPL so input doesn't fight stdin readers.
type CLIApprover struct {
	Reader *bufio.Reader
	Writer io.Writer
}

func NewCLIApprover(r *bufio.Reader, w io.Writer) *CLIApprover {
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

// RunCLI runs a simple stdin REPL. Each line of input is one user turn; agent
// events stream to stdout while the model thinks. The shared bufio.Reader is
// reused by CLIApprover so destructive-command prompts don't fight stdin.
func RunCLI(ctx context.Context, ag *agent.Agent, rc *skills.RegistryClient, sessionID string, r *bufio.Reader) error {
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

	fmt.Printf("cc-agent ready. session=%s. type ':help' for commands, 'exit' to quit.\n", sessionID)
	if r == nil {
		r = bufio.NewReader(os.Stdin)
	}
	for {
		fmt.Print("\nyou> ")
		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
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
			if err := handleSlashCommand(ctx, ag, rc, sessionID, line); err != nil {
				fmt.Fprintf(os.Stderr, "command error: %v\n", err)
			}
			continue
		}
		if _, err := ag.Run(ctx, sessionID, line); err != nil {
			fmt.Fprintf(os.Stderr, "run error: %v\n", err)
		}
	}
}

// handleSlashCommand dispatches REPL meta commands. Currently:
//   :help                         show commands
//   :skills                       list loaded skills
//   :reflect <name> [description] distill the current session into a skill
func handleSlashCommand(ctx context.Context, ag *agent.Agent, rc *skills.RegistryClient, sessionID, line string) error {
	parts := strings.Fields(line)
	cmd := parts[0]
	switch cmd {
	case ":help", ":?":
		fmt.Println(`commands:
  :help                            show this help
  :skills                          list loaded skills
  :reflect <name> [description]    distill current session into a skill
  :registry [search]               list team skills
  :publish <name>                  push a local skill to the team registry
  exit | quit                      leave`)
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
	default:
		return fmt.Errorf("unknown command %q (try :help)", cmd)
	}
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
