// cc-agent is the standalone server-ops agent: it owns its own LLM loop,
// tool registry, and persistence. It can run as a local REPL, a small HTTP
// service, or (future) attach to a cc-control plane.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"cc-agent/internal/agent"
	"cc-agent/internal/config"
	"cc-agent/internal/llm"
	"cc-agent/internal/memory"
	"cc-agent/internal/skills"
	"cc-agent/internal/tools"
	"cc-agent/internal/transport"
	"cc-agent/internal/transport/control"
	"cc-agent/internal/version"
)

func main() {
	var (
		cfgPath    = flag.String("config", "", "JSON config file (optional)")
		provider   = flag.String("provider", "", "override provider (anthropic|openai|deepseek)")
		model      = flag.String("model", "", "override model name")
		cwd        = flag.String("cwd", "", "agent working directory (default $PWD)")
		memoryPath = flag.String("memory", "", "SQLite path for persistent sessions; empty = in-memory")
		httpAddr   = flag.String("http", "", "optional HTTP API listen addr (e.g. :19090)")
		controlURL = flag.String("control-url", "", "optional cc-control WS URL")
		agentToken = flag.String("agent-token", "", "agent token (with -control-url)")
		serverID   = flag.String("server-id", "", "agent server id (with -control-url)")
		sessionID  = flag.String("session", "default", "session id for the CLI REPL")
		skillsDir  = flag.String("skills-dir", "", "directory of skill JSON files")
		listTools  = flag.Bool("list-tools", false, "print registered tools as JSON and exit (no API key needed)")
		showVer    = flag.Bool("version", false, "print version and exit")
		fullPerm   = flag.Bool("full-permission", false, "skip approval prompts for destructive commands (yolo mode)")
		denyDanger = flag.Bool("deny-destructive", false, "auto-deny destructive commands (no prompt, safe for non-interactive use)")
		approvalTO = flag.String("approval-timeout", "", "how long to wait for an operator approval in -control-url mode (e.g. 30s, 5m, 1h). Default 5m.")
	)
	flag.Parse()

	if *showVer {
		fmt.Println(version.String())
		return
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if *provider != "" {
		cfg.Provider = *provider
	}
	if *model != "" {
		cfg.Model = *model
	}
	if *cwd != "" {
		cfg.Cwd = *cwd
	}
	if *memoryPath != "" {
		cfg.MemoryPath = *memoryPath
	}
	if *httpAddr != "" {
		cfg.HTTPListen = *httpAddr
	}
	if *controlURL != "" {
		cfg.ControlURL = *controlURL
	}
	if *agentToken != "" {
		cfg.AgentToken = *agentToken
	}
	if *serverID != "" {
		cfg.ServerID = *serverID
	}
	if *skillsDir != "" {
		cfg.SkillsDir = *skillsDir
	}
	if *approvalTO != "" {
		cfg.ApprovalTimeout = *approvalTO
	}

	if *listTools {
		toolReg := tools.DefaultRegistry(cfg.Cwd, cfg.AllowedRoots)
		printTools(toolReg)
		return
	}

	provider2, err := llm.New(llm.Config{
		Provider: cfg.Provider,
		APIKey:   cfg.APIKey,
		BaseURL:  cfg.BaseURL,
		Timeout:  cfg.TimeoutDuration(),
	})
	if err != nil {
		log.Fatalf("llm provider: %v", err)
	}

	mem, err := openMemory(cfg.MemoryPath)
	if err != nil {
		log.Fatalf("memory: %v", err)
	}
	defer mem.Close()

	skillReg := skills.NewRegistry()
	if cfg.SkillsDir != "" {
		localDir := filepath.Join(cfg.SkillsDir, "local")
		teamDir := filepath.Join(cfg.SkillsDir, "team")
		// If neither subdirectory exists, fall back to flat layout (backwards-compat).
		_, localErr := os.Stat(localDir)
		_, teamErr := os.Stat(teamDir)
		if os.IsNotExist(localErr) && os.IsNotExist(teamErr) {
			if err := skillReg.LoadDir(cfg.SkillsDir); err != nil {
				log.Printf("skills: %v (continuing without)", err)
			}
		} else {
			if err := skills.LoadTwoPhase(skillReg, localDir, teamDir); err != nil {
				log.Printf("skills: %v (continuing without)", err)
			}
		}
	}

	// teamDir is the on-disk location for team-scope (registry-delivered)
	// skills. Shared by the registry HTTP client (CLI-driven installs) and
	// the WS install_skill_request handler (UI-driven installs).
	teamDir := ""
	if cfg.SkillsDir != "" {
		teamDir = filepath.Join(cfg.SkillsDir, "team")
	}

	// RegistryClient talks to cc-control's /api/registry/* endpoints for
	// publish/install/list/history. Built only when both ControlHTTPURL and
	// AgentToken are set.
	var registryClient *skills.RegistryClient
	if cfg.ControlHTTPURL != "" && cfg.AgentToken != "" {
		registryClient = &skills.RegistryClient{
			BaseURL:    cfg.ControlHTTPURL,
			AgentToken: cfg.AgentToken,
			ServerID:   cfg.ServerID,
			TeamDir:    teamDir,
		}
	}

	toolReg := tools.DefaultRegistry(cfg.Cwd, cfg.AllowedRoots)
	stdinReader := bufio.NewReader(os.Stdin)
	bashTool, _ := toolReg.Get("bash")
	if b, ok := bashTool.(*tools.Bash); ok {
		switch {
		case *fullPerm:
			b.Approver = tools.AlwaysApprove{}
			fmt.Fprintln(os.Stderr, "\033[31m[!] full-permission mode: destructive commands run without prompt\033[0m")
		case *denyDanger:
			b.Approver = tools.AlwaysDeny{}
		default:
			b.Approver = transport.NewCLIApprover(stdinReader, os.Stderr)
		}
	}
	ag := agent.New(provider2, toolReg, mem, agent.Options{
		Model:         cfg.Model,
		MaxIterations: cfg.MaxIterations,
		MaxTokens:     cfg.MaxTokens,
		SystemPrompt:  cfg.SystemPrompt,
	})
	if cfg.SkillsDir != "" {
		ag.SetSkills(skillReg, cfg.SkillsDir, &skills.LLMReflector{Provider: provider2, Model: cfg.Model})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-sigCh; cancel() }()

	if cfg.ControlURL != "" {
		client, err := control.New(control.Options{
			URL:          cfg.ControlURL,
			Token:        cfg.AgentToken,
			ServerID:     cfg.ServerID,
			AllowedRoots: cfg.AllowedRoots,
			Tags:         []string{"cc-agent"},
			TeamDir:      teamDir,
			Agent:        ag,
		})
		if err != nil {
			log.Fatalf("control: %v", err)
		}
		// In control-bridge mode, replace the bash tool's approver with one
		// that routes through cc-control to the operator's UI. The CLI / deny
		// approvers chosen earlier by -full-permission / -deny-destructive
		// remain the *fallback* default; -full-permission still wins because
		// remote approval is unnecessary when the operator has opted into
		// yolo mode.
		if !*fullPerm && !*denyDanger {
			if b, ok := bashTool.(*tools.Bash); ok {
				approver := control.NewRemoteApproverWithTimeout(client, cfg.ApprovalTimeoutDuration())
				client.SetApprover(approver)
				b.Approver = approver
				fmt.Fprintf(os.Stderr, "[approval] UI-routed approver: timeout=%s\n", cfg.ApprovalTimeoutDuration())
			}
		}
		go func() {
			if err := client.Run(ctx); err != nil {
				log.Printf("control: %v", err)
			}
		}()
		fmt.Printf("[control] registering with %s as %s\n", cfg.ControlURL, cfg.ServerID)
	}
	if cfg.HTTPListen != "" {
		go func() {
			h := transport.NewHTTPServer(cfg.HTTPListen, ag)
			fmt.Printf("[http] listening on %s\n", cfg.HTTPListen)
			if err := h.ListenAndServe(ctx); err != nil {
				log.Printf("[http] %v", err)
			}
		}()
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Printf("provider=%s model=%s cwd=%s tools=%d skills=%d\n",
		cfg.Provider, cfg.Model, cfg.Cwd, len(toolReg.All()), len(skillReg.Names()))

	// Skip the REPL when running as a remote bridge or daemon. The process
	// stays up driven by the control-url / http server until ctx is canceled.
	if cfg.ControlURL != "" || cfg.HTTPListen != "" {
		fmt.Println("running as daemon (no REPL); send SIGINT/SIGTERM to stop.")
		<-ctx.Done()
		fmt.Println("shutting down.")
		return
	}

	if err := transport.RunCLI(ctx, ag, registryClient, *sessionID, stdinReader); err != nil {
		log.Fatalf("cli: %v", err)
	}
	fmt.Println("bye.")
}

func openMemory(path string) (memory.Store, error) {
	if path == "" {
		return memory.NewInMemory(), nil
	}
	return memory.OpenSQLite(path)
}

func printTools(reg *tools.Registry) {
	type toolView struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		InputSchema map[string]any `json:"input_schema"`
	}
	out := make([]toolView, 0)
	for _, t := range reg.All() {
		out = append(out, toolView{
			Name: t.Name(), Description: t.Description(), InputSchema: t.InputSchema(),
		})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}
