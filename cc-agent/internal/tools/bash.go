package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"cc-agent/internal/host"
)

// Bash runs a shell command. Constructed with a fixed cwd and an allow-list of
// roots; commands themselves are not parsed (the LLM may pipe / redirect).
// Hard caps prevent runaway processes.
//
// If Approver is set and DangerousReason returns non-empty for a command, the
// approver is consulted before exec. A miss is reported back to the model as
// a string result (not an error) so it can react and choose a safer path.
type Bash struct {
	Cwd            string
	DefaultTimeout time.Duration
	MaxOutputBytes int
	BlockedSubstr  []string // commands containing any of these strings are rejected outright
	Approver       Approver
	Shell          host.Shell
}

func NewBash(cwd string) *Bash {
	return &Bash{
		Cwd:            cwd,
		DefaultTimeout: 60 * time.Second,
		MaxOutputBytes: 64 * 1024,
		BlockedSubstr:  nil,
		Shell:          host.CurrentShell(),
	}
}

func (*Bash) Name() string { return "bash" }

func (b *Bash) Description() string {
	return "Execute a shell command. Use for system inspection, log queries, " +
		"package status, etc. Output is truncated to a fixed size; chain commands " +
		"with shell pipelines as needed. Working directory is fixed by the agent. " +
		"Commands run via " + b.Shell.DisplayName() + "."
}

func (b *Bash) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Shell command to run via " + b.Shell.DisplayName() + ".",
			},
			"timeout_sec": map[string]any{
				"type":        "integer",
				"description": "Optional timeout in seconds. Default 60, max 600.",
			},
		},
		"required": []string{"command"},
	}
}

func (b *Bash) Run(ctx context.Context, input map[string]any) (string, error) {
	cmdStr, err := MustString(input, "command")
	if err != nil {
		return "", err
	}
	for _, blocked := range b.BlockedSubstr {
		if blocked != "" && strings.Contains(cmdStr, blocked) {
			return "", fmt.Errorf("command blocked: contains %q", blocked)
		}
	}
	if reason := DangerousReason(cmdStr); reason != "" && b.Approver != nil {
		ok, err := b.Approver.Approve(ctx, cmdStr, reason)
		if err != nil {
			return fmt.Sprintf("approval error: %v\ncommand: %s\nreason: %s\n", err, cmdStr, reason), nil
		}
		if !ok {
			return fmt.Sprintf("DENIED by operator (reason: %s).\ncommand: %s\nthe operator declined to run this. consider a safer alternative or ask before retrying.\n",
				reason, cmdStr), nil
		}
	}
	timeoutSec := OptInt(input, "timeout_sec", int(b.DefaultTimeout/time.Second))
	if timeoutSec <= 0 {
		timeoutSec = int(b.DefaultTimeout / time.Second)
	}
	if timeoutSec > 600 {
		timeoutSec = 600
	}
	cctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	name, args := b.Shell.CommandLine(cmdStr)
	cmd := exec.CommandContext(cctx, name, args...)
	cmd.Dir = b.Cwd
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb

	startErr := cmd.Run()
	stdout := truncate(out.Bytes(), b.MaxOutputBytes)
	stderr := truncate(errb.Bytes(), b.MaxOutputBytes)

	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	timedOut := cctx.Err() == context.DeadlineExceeded

	var sb strings.Builder
	fmt.Fprintf(&sb, "exit_code: %d\n", exitCode)
	if timedOut {
		fmt.Fprintln(&sb, "timed_out: true")
	}
	if len(stdout) > 0 {
		sb.WriteString("--- stdout ---\n")
		sb.Write(stdout)
		if !bytes.HasSuffix(stdout, []byte("\n")) {
			sb.WriteByte('\n')
		}
	}
	if len(stderr) > 0 {
		sb.WriteString("--- stderr ---\n")
		sb.Write(stderr)
		if !bytes.HasSuffix(stderr, []byte("\n")) {
			sb.WriteByte('\n')
		}
	}
	if startErr != nil && !timedOut && exitCode != -1 {
		fmt.Fprintf(&sb, "--- error ---\n%v\n", startErr)
	}
	return sb.String(), nil
}

func truncate(b []byte, maxBytes int) []byte {
	if maxBytes <= 0 || len(b) <= maxBytes {
		return b
	}
	cut := b[:maxBytes]
	suffix := fmt.Sprintf("\n... [truncated %d bytes] ...\n", len(b)-maxBytes)
	return append(cut, []byte(suffix)...)
}
