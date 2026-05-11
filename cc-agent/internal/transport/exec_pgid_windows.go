//go:build windows

package transport

import "os/exec"

// configureProcessGroup is a no-op on Windows. The process-group SIGKILL
// trick used on Unix has no direct equivalent here; cmd.Run's default
// cancel (Process.Kill on the immediate child) is the best we can do.
func configureProcessGroup(_ *exec.Cmd) {}
