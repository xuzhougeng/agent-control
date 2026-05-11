package config

import (
	"strings"
	"testing"

	"cc-agent/internal/host"
)

func TestDefaultSystemPromptForHost_IncludesWindowsPowerShellContext(t *testing.T) {
	prompt := DefaultSystemPromptForHost(host.Context{
		GOOS:   "windows",
		GOARCH: "amd64",
		Shell: host.Shell{
			Kind:    host.ShellPowerShell,
			Command: "powershell.exe",
		},
	})
	for _, want := range []string{
		"windows/amd64",
		"PowerShell",
		"Get-ChildItem",
		"Do not assume Linux-only commands",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "maintain Linux servers") {
		t.Fatalf("default prompt should not hard-code Linux-only hosts:\n%s", prompt)
	}
}
