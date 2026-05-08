package claudecli

import "strings"

// SupportsSessionFlags reports whether the executable path looks like a Claude CLI
// command that understands --session-id/--resume.
func SupportsSessionFlags(cmdPath string) bool {
	base := normalizeCommandBase(cmdPath)
	if base == "" {
		return false
	}
	return base == "claude" || base == "claude-code"
}

func normalizeCommandBase(cmdPath string) string {
	cmdPath = strings.TrimSpace(cmdPath)
	if cmdPath == "" {
		return ""
	}
	if idx := strings.LastIndexAny(cmdPath, `/\`); idx >= 0 {
		cmdPath = cmdPath[idx+1:]
	}
	cmdPath = strings.ToLower(strings.TrimSpace(cmdPath))
	for _, ext := range []string{".exe", ".cmd", ".bat", ".ps1", ".sh"} {
		cmdPath = strings.TrimSuffix(cmdPath, ext)
	}
	return cmdPath
}
