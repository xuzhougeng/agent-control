//go:build !windows

package transport

import (
	"os/exec"
	"syscall"
)

// configureProcessGroup puts the spawned shell (and its descendants) in a
// fresh process group, and overrides cmd.Cancel so context cancellation
// SIGKILLs the entire group instead of just the immediate child.
//
// Without this, cmd.Run can block long after sh has been killed: when
// cmd.Stdout is a non-*os.File (e.g. io.MultiWriter), Go's exec package
// pipes output through a goroutine that waits for the pipe to close,
// which only happens when every inheritor releases its fd. So a SIGKILL
// to sh leaves a grandchild like `sleep 10` holding the pipe open, and
// the REPL appears wedged for the full sleep duration after ESC.
func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative PID targets the whole process group whose PGID equals
		// the leader's PID. Errors here are best-effort: the group may
		// already be gone.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return nil
	}
}
