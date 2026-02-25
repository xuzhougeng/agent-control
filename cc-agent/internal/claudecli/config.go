package claudecli

import (
	"os"
	"strings"
)

type Config struct {
	Cmd                string
	PermissionMode     string
	AllowedTools       string
	DisallowedTools    string
	Model              string
	Effort             string
	SystemPrompt       string
	AppendSystemPrompt string
	Betas              string
	AddDirs            []string
	TimeoutMS          int
}

func LoadConfigFromEnv() Config {
	cfg := Config{
		Cmd:            strings.TrimSpace(getenv("CC_CLAUDE_CMD", "claude")),
		PermissionMode: strings.TrimSpace(getenv("CC_CLAUDE_PERMISSION_MODE", "dontAsk")),
		AllowedTools:   strings.TrimSpace(os.Getenv("CC_CLAUDE_ALLOWED_TOOLS")),
		DisallowedTools: strings.TrimSpace(
			os.Getenv("CC_CLAUDE_DISALLOWED_TOOLS"),
		),
		Model:              strings.TrimSpace(os.Getenv("CC_CLAUDE_MODEL")),
		Effort:             strings.TrimSpace(os.Getenv("CC_CLAUDE_EFFORT")),
		SystemPrompt:       strings.TrimSpace(os.Getenv("CC_CLAUDE_SYSTEM_PROMPT")),
		AppendSystemPrompt: strings.TrimSpace(os.Getenv("CC_CLAUDE_APPEND_SYSTEM_PROMPT")),
		Betas:              strings.TrimSpace(os.Getenv("CC_CLAUDE_BETAS")),
		AddDirs:            splitCSV(os.Getenv("CC_CLAUDE_ADD_DIR")),
		TimeoutMS:          parseInt(os.Getenv("CC_CLAUDE_TIMEOUT_MS")),
	}
	return cfg
}

func BaseArgs(cfg Config) []string {
	args := []string{
		"-p",
		"--verbose",
		"--input-format=stream-json",
		"--output-format=stream-json",
	}
	addArg(&args, "--permission-mode", cfg.PermissionMode)
	addArg(&args, "--allowed-tools", cfg.AllowedTools)
	addArg(&args, "--disallowed-tools", cfg.DisallowedTools)
	addArg(&args, "--model", cfg.Model)
	addArg(&args, "--effort", cfg.Effort)
	addArg(&args, "--system-prompt", cfg.SystemPrompt)
	addArg(&args, "--append-system-prompt", cfg.AppendSystemPrompt)
	addArg(&args, "--betas", cfg.Betas)
	for _, dir := range cfg.AddDirs {
		addArg(&args, "--add-dir", dir)
	}
	return args
}

func addArg(args *[]string, flag, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	*args = append(*args, flag, value)
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func parseInt(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	var n int
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + int(ch-'0')
	}
	return n
}
