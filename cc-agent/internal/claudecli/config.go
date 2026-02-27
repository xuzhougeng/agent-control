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
	ProfileFile        string
	InjectRuntimeCtx   bool
}

func LoadConfigFromEnv() Config {
	cfg := Config{
		Cmd:                strings.TrimSpace(getenv("CC_CLAUDE_CMD", "claude")),
		PermissionMode:     strings.TrimSpace(getenv("CC_CLAUDE_PERMISSION_MODE", "dontAsk")),
		AllowedTools:       strings.TrimSpace(os.Getenv("CC_CLAUDE_ALLOWED_TOOLS")),
		DisallowedTools:    strings.TrimSpace(os.Getenv("CC_CLAUDE_DISALLOWED_TOOLS")),
		Model:              strings.TrimSpace(os.Getenv("CC_CLAUDE_MODEL")),
		Effort:             strings.TrimSpace(os.Getenv("CC_CLAUDE_EFFORT")),
		SystemPrompt:       strings.TrimSpace(os.Getenv("CC_CLAUDE_SYSTEM_PROMPT")),
		AppendSystemPrompt: strings.TrimSpace(os.Getenv("CC_CLAUDE_APPEND_SYSTEM_PROMPT")),
		Betas:              strings.TrimSpace(os.Getenv("CC_CLAUDE_BETAS")),
		AddDirs:            splitCSV(os.Getenv("CC_CLAUDE_ADD_DIR")),
		TimeoutMS:          parseInt(os.Getenv("CC_CLAUDE_TIMEOUT_MS")),
		ProfileFile:        strings.TrimSpace(os.Getenv("CC_CLAUDE_PROFILE_FILE")),
		InjectRuntimeCtx:   parseBool(getenv("CC_CLAUDE_INJECT_RUNTIME_CONTEXT", "1"), true),
	}

	appendParts := make([]string, 0, 3)
	appendParts = appendPart(appendParts, cfg.AppendSystemPrompt)
	if cfg.InjectRuntimeCtx {
		appendParts = appendPart(appendParts, buildRuntimeContext(cfg))
	}
	if profileText := loadProfileText(cfg.ProfileFile); profileText != "" {
		appendParts = appendPart(appendParts, "## Profile Instructions\n"+profileText)
	}
	cfg.AppendSystemPrompt = strings.Join(appendParts, "\n\n")
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

func buildRuntimeContext(cfg Config) string {
	lines := []string{
		"## Runtime Context",
		"- session_type: chat",
		"- agent_os: " + getenv("CC_AGENT_OS", "unknown"),
		"- agent_arch: " + getenv("CC_AGENT_ARCH", "unknown"),
		"- server_id: " + getenv("CC_AGENT_SERVER_ID", "unknown"),
		"- hostname: " + getenv("CC_AGENT_HOSTNAME", "unknown"),
		"- chat_session_id: " + getenv("CC_CHAT_SESSION_ID", "unknown"),
		"- cwd: " + getenv("CC_AGENT_SESSION_CWD", "unknown"),
		"- allow_roots: " + valueOrUnknown(os.Getenv("CC_AGENT_ALLOW_ROOTS")),
		"- tags: " + valueOrUnknown(os.Getenv("CC_AGENT_TAGS")),
		"",
		"## Capability Boundaries",
		"- permission_mode: " + valueOrUnknown(cfg.PermissionMode),
		"- allowed_tools: " + valueOrUnknown(cfg.AllowedTools),
		"- disallowed_tools: " + valueOrUnknown(cfg.DisallowedTools),
		"- add_dirs: " + valueOrUnknown(strings.Join(cfg.AddDirs, ",")),
		"",
		"Follow the capability boundaries above strictly when replying.",
	}
	return strings.Join(lines, "\n")
}

func loadProfileText(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func appendPart(parts []string, text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return parts
	}
	return append(parts, text)
}

func valueOrUnknown(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "unknown"
	}
	return v
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

func parseBool(raw string, fallback bool) bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
