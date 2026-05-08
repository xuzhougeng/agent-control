// Package config loads cc-agent runtime settings from a JSON file plus
// environment overrides. JSON is chosen over YAML to avoid an extra dep.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const DefaultSystemPrompt = `You are cc-agent, a server operations assistant.
You help diagnose, configure, and maintain Linux servers via a small set of
local tools (bash, read, write, grep, glob, sysinfo, proclist, logtail).

Operating principles:
- Prefer read-only inspection (sysinfo / proclist / logtail / grep) before any
  mutation.
- Before running a destructive or service-affecting command, state what you
  intend to do and why.
- Quote concrete paths, PIDs, exit codes, and log excerpts in your reasoning;
  do not fabricate them.
- When unsure, ask the user before continuing.
- Keep responses tight; output is read by an operator on a terminal.
`

type Config struct {
	// Provider: "anthropic" | "openai" | "deepseek" | "qwen" | "openai-compat".
	Provider string `json:"provider"`
	Model    string `json:"model"`
	APIKey   string `json:"api_key"`
	BaseURL  string `json:"base_url"`
	Timeout  string `json:"timeout"` // e.g. "120s"

	// Agent runtime.
	Cwd           string   `json:"cwd"`
	AllowedRoots  []string `json:"allowed_roots"`
	MaxIterations int      `json:"max_iterations"`
	MaxTokens     int      `json:"max_tokens"`
	SystemPrompt  string   `json:"system_prompt"`
	SkillsDir     string   `json:"skills_dir"`

	// Persistence.
	MemoryPath string `json:"memory_path"` // empty = in-memory only

	// Optional integration.
	ControlURL  string `json:"control_url"`
	AgentToken  string `json:"agent_token"`
	ServerID    string `json:"server_id"`
	HTTPListen  string `json:"http_listen"` // e.g. ":19090"

	// ApprovalTimeout is how long RemoteApprover (cc-control bridge mode) waits
	// for a human to click Approve/Reject before auto-denying. Empty/invalid
	// → 5 minutes. Format is a Go duration string ("30s", "10m", "1h").
	ApprovalTimeout string `json:"approval_timeout"`
}

// Load reads cfg from path (may be empty), applies env overrides, then sets
// safe defaults.
func Load(path string) (*Config, error) {
	cfg := &Config{}
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config %s: %w", path, err)
		}
		if err := json.Unmarshal(raw, cfg); err != nil {
			return nil, fmt.Errorf("parse config %s: %w", path, err)
		}
	}
	cfg.applyEnv()
	cfg.applyDefaults()
	return cfg, cfg.validate()
}

func (c *Config) applyEnv() {
	if v := os.Getenv("CC_AGENT_PROVIDER"); v != "" {
		c.Provider = v
	}
	if v := os.Getenv("CC_AGENT_MODEL"); v != "" {
		c.Model = v
	}
	if v := os.Getenv("CC_AGENT_BASE_URL"); v != "" {
		c.BaseURL = v
	}
	if v := os.Getenv("CC_AGENT_API_KEY"); v != "" {
		c.APIKey = v
	}
	if v := os.Getenv("CC_AGENT_MEMORY_PATH"); v != "" {
		c.MemoryPath = v
	}
	if v := os.Getenv("CC_AGENT_SKILLS_DIR"); v != "" {
		c.SkillsDir = v
	}
	if v := os.Getenv("CC_AGENT_APPROVAL_TIMEOUT"); v != "" {
		c.ApprovalTimeout = v
	}
}

// ApprovalTimeoutDuration parses ApprovalTimeout into a time.Duration; falls
// back to 5 minutes when empty or malformed. Negative or zero durations are
// treated as the default.
func (c *Config) ApprovalTimeoutDuration() time.Duration {
	if c.ApprovalTimeout == "" {
		return 5 * time.Minute
	}
	d, err := time.ParseDuration(c.ApprovalTimeout)
	if err != nil || d <= 0 {
		return 5 * time.Minute
	}
	return d
}

func (c *Config) applyDefaults() {
	if c.Provider == "" {
		c.Provider = "anthropic"
	}
	if c.Model == "" {
		switch c.Provider {
		case "anthropic":
			c.Model = "claude-sonnet-4-6"
		case "openai", "openai-compat":
			c.Model = "gpt-4o-mini"
		case "deepseek":
			c.Model = "deepseek-chat"
		}
	}
	if c.MaxIterations == 0 {
		c.MaxIterations = 12
	}
	if c.MaxTokens == 0 {
		c.MaxTokens = 4096
	}
	if c.SystemPrompt == "" {
		c.SystemPrompt = DefaultSystemPrompt
	}
	if c.Cwd == "" {
		if pwd, err := os.Getwd(); err == nil {
			c.Cwd = pwd
		}
	}
	if c.Cwd != "" {
		c.Cwd, _ = filepath.Abs(c.Cwd)
	}
}

func (c *Config) validate() error {
	if c.Cwd == "" {
		return errors.New("cwd is required")
	}
	return nil
}

// TimeoutDuration parses Timeout into a time.Duration; falls back to 300s.
// Default chosen to accommodate reasoning models (DeepSeek v4-pro, qwen-thinking)
// that can take 1-3 minutes per turn when chain-of-thought is enabled.
func (c *Config) TimeoutDuration() time.Duration {
	if c.Timeout == "" {
		return 300 * time.Second
	}
	d, err := time.ParseDuration(c.Timeout)
	if err != nil || d <= 0 {
		return 300 * time.Second
	}
	return d
}
