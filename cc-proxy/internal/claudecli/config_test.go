package claudecli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigFromEnv_AppendsRuntimeContextAndProfile(t *testing.T) {
	profilePath := filepath.Join(t.TempDir(), "profile.md")
	if err := os.WriteFile(profilePath, []byte("Use concise responses."), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	t.Setenv("CC_CLAUDE_APPEND_SYSTEM_PROMPT", "Base append prompt")
	t.Setenv("CC_CLAUDE_PROFILE_FILE", profilePath)
	t.Setenv("CC_CLAUDE_INJECT_RUNTIME_CONTEXT", "1")
	t.Setenv("CC_AGENT_SERVER_ID", "srv-1")
	t.Setenv("CC_AGENT_HOSTNAME", "host-1")
	t.Setenv("CC_AGENT_OS", "linux")
	t.Setenv("CC_AGENT_ARCH", "amd64")
	t.Setenv("CC_CHAT_SESSION_ID", "chat-1")
	t.Setenv("CC_AGENT_SESSION_CWD", "/workspace")
	t.Setenv("CC_AGENT_ALLOW_ROOTS", "/workspace,/tmp")
	t.Setenv("CC_AGENT_TAGS", "prod,chat")
	t.Setenv("CC_CLAUDE_PERMISSION_MODE", "dontAsk")
	t.Setenv("CC_CLAUDE_ALLOWED_TOOLS", "Read Edit")
	t.Setenv("CC_CLAUDE_DISALLOWED_TOOLS", "Bash(rm:*)")
	t.Setenv("CC_CLAUDE_ADD_DIR", "/workspace,/workspace/docs")

	cfg := LoadConfigFromEnv()
	got := cfg.AppendSystemPrompt
	for _, want := range []string{
		"Base append prompt",
		"## Runtime Context",
		"server_id: srv-1",
		"cwd: /workspace",
		"allowed_tools: Read Edit",
		"## Profile Instructions",
		"Use concise responses.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("append prompt missing %q, got:\n%s", want, got)
		}
	}
}

func TestLoadConfigFromEnv_DisableRuntimeContext(t *testing.T) {
	t.Setenv("CC_CLAUDE_INJECT_RUNTIME_CONTEXT", "0")
	t.Setenv("CC_CLAUDE_APPEND_SYSTEM_PROMPT", "Only base")

	cfg := LoadConfigFromEnv()
	got := cfg.AppendSystemPrompt
	if strings.Contains(got, "## Runtime Context") {
		t.Fatalf("runtime context should be disabled, got:\n%s", got)
	}
	if !strings.Contains(got, "Only base") {
		t.Fatalf("base append prompt should be preserved, got:\n%s", got)
	}
}

func TestLoadConfigFromEnv_MissingProfileFileIgnored(t *testing.T) {
	t.Setenv("CC_CLAUDE_PROFILE_FILE", filepath.Join(t.TempDir(), "missing.md"))
	t.Setenv("CC_CLAUDE_INJECT_RUNTIME_CONTEXT", "0")
	t.Setenv("CC_CLAUDE_APPEND_SYSTEM_PROMPT", "")

	cfg := LoadConfigFromEnv()
	if strings.Contains(cfg.AppendSystemPrompt, "Profile Instructions") {
		t.Fatalf("missing profile file should not inject section, got:\n%s", cfg.AppendSystemPrompt)
	}
}
