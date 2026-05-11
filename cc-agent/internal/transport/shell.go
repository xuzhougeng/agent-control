package transport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// execShell runs cmdStr through /bin/sh -c, streaming combined stdout+stderr
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
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
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

// shellEntry is one !! invocation captured for later folding into the next
// real user message. Strings already include any [exit:] / [timed out]
// markers added by execShell.
type shellEntry struct {
	cmd string
	out string
}

// pendingShell is a single-session FIFO of !! captures. Not goroutine-safe
// because RunCLI is single-threaded — the REPL goroutine is the only writer
// and reader.
type pendingShell struct {
	entries []shellEntry
}

func (p *pendingShell) push(cmd, out string) {
	p.entries = append(p.entries, shellEntry{cmd: cmd, out: out})
}

func (p *pendingShell) take() []shellEntry {
	out := p.entries
	p.entries = nil
	return out
}

func (p *pendingShell) empty() bool { return len(p.entries) == 0 }

// foldShellContext renders a drained pendingShell buffer + the operator's
// fresh line into the single user message that gets handed to agent.Run.
// When entries is empty the raw line is returned unchanged so a session with
// zero !! usage is byte-identical to today.
func foldShellContext(entries []shellEntry, userLine string) string {
	if len(entries) == 0 {
		return userLine
	}
	var sb strings.Builder
	for i, e := range entries {
		if i > 0 {
			sb.WriteString("---\n")
		}
		sb.WriteString("[user-shell] $ ")
		sb.WriteString(e.cmd)
		sb.WriteByte('\n')
		out := e.out
		if out == "" {
			out = "(no output)\n"
		} else if !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		sb.WriteString(out)
	}
	sb.WriteString("===\n")
	sb.WriteString(userLine)
	return sb.String()
}
