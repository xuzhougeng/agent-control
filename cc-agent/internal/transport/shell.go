package transport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
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
