package transport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"cc-agent/internal/host"
)

// execShell runs cmdStr through the host shell, streaming combined stdout+stderr
// to live (typically the readline Stdout) and capturing them into a returned
// string. The captured form is truncated to maxBytes with a marker, so the
// REPL buffer can safely fold huge command output into a memory write
// without bloating the next provider request. Live streaming is uncapped.
//
// timeout is enforced as a hard cap on top of ctx; when timeout <= 0 the
// caller's ctx alone gates the run.
//
// All run failures (non-zero exit, missing shell, permission errors) are
// folded into the captured string; the returned error is always nil for
// shell-level failures, so callers can rely on err == nil for any
// shell-side problem. A non-nil error indicates an internal misuse.
func execShell(ctx context.Context, cmdStr, cwd string, live io.Writer, timeout time.Duration, maxBytes int) (string, error) {
	return execShellWithShell(ctx, host.CurrentShell(), cmdStr, cwd, live, timeout, maxBytes)
}

func execShellWithShell(ctx context.Context, shell host.Shell, cmdStr, cwd string, live io.Writer, timeout time.Duration, maxBytes int) (string, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	name, args := shell.CommandLine(cmdStr)
	cmd := exec.CommandContext(ctx, name, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	// On Unix, put the shell in its own process group so context
	// cancellation can SIGKILL the whole tree. Otherwise a grandchild
	// (e.g. `sleep 10`) keeps the inherited stdout pipe open and
	// cmd.Run blocks until it exits naturally, wedging the REPL.
	configureProcessGroup(cmd)
	var captured bytes.Buffer
	w := io.MultiWriter(live, &captured)
	cmd.Stdout = w
	cmd.Stderr = w
	runErr := cmd.Run()
	out := truncateBytes(captured.Bytes(), maxBytes)
	if ctx.Err() == context.DeadlineExceeded {
		return string(out) + "\n[timed out]\n", nil
	}
	if runErr != nil {
		return string(out) + fmt.Sprintf("\n[exit: %v]\n", runErr), nil
	}
	return string(out), nil
}

func truncateBytes(b []byte, maxBytes int) []byte {
	if maxBytes <= 0 || len(b) <= maxBytes {
		return b
	}
	cut := b[:maxBytes]
	return append(cut, []byte(fmt.Sprintf("\n…(+%d more bytes)\n", len(b)-maxBytes))...)
}

const (
	labelShell  = "user-shell"
	labelAttach = "user-attach"
)

// injectEntry is one staged item — either a !! shell capture or an @ file
// attach — waiting to be folded into the next real user message. The label
// drives the fold header (`[user-shell]` or `[user-attach]`); head is the
// post-bracket title line ("$ <cmd>" or "@<path>"); body is the multi-line
// content (command output or file contents, with any markers already baked in
// by the producer).
type injectEntry struct {
	label string // "user-shell" | "user-attach"
	head  string
	body  string
}

// pendingInject is a single-session FIFO of staged !! and @ entries. Not
// goroutine-safe because RunCLI is single-threaded — the REPL goroutine is
// the only writer and reader.
type pendingInject struct {
	entries []injectEntry
}

func (p *pendingInject) pushShell(cmd, out string) {
	p.entries = append(p.entries, injectEntry{label: labelShell, head: "$ " + cmd, body: out})
}

// newAttachEntry builds the canonical injectEntry for an @ file attach.
// It is the single source of truth for the attach head format and the
// empty-body fallback, so the stage form (pushAttach) and inline form
// (runAttach) cannot drift.
func newAttachEntry(path, content string, truncated bool) injectEntry {
	if content == "" {
		content = "(empty file)\n"
	}
	head := "@" + path
	if truncated {
		head += " (truncated)"
	}
	return injectEntry{label: labelAttach, head: head, body: content}
}

func (p *pendingInject) pushAttach(path, content string, truncated bool) {
	p.entries = append(p.entries, newAttachEntry(path, content, truncated))
}

func (p *pendingInject) take() []injectEntry {
	out := p.entries
	p.entries = nil
	return out
}

func (p *pendingInject) empty() bool { return len(p.entries) == 0 }

// foldInject renders a drained pendingInject buffer + the operator's fresh
// line into the single user message that gets handed to agent.Run. Shell
// captures and file attaches share one format: a "[label] head" line, then
// the body, with "---" between entries and "===" before the user line.
// When entries is empty the raw line is returned unchanged so a session
// with zero injections is byte-identical to today.
func foldInject(entries []injectEntry, userLine string) string {
	if len(entries) == 0 {
		return userLine
	}
	var sb strings.Builder
	for i, e := range entries {
		if i > 0 {
			sb.WriteString("---\n")
		}
		sb.WriteString("[")
		sb.WriteString(e.label)
		sb.WriteString("] ")
		sb.WriteString(e.head)
		sb.WriteByte('\n')
		body := e.body
		if body == "" {
			body = "(no output)\n"
		} else if !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		sb.WriteString(body)
	}
	sb.WriteString("===\n")
	sb.WriteString(userLine)
	return sb.String()
}
