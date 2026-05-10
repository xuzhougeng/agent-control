//go:build !windows

package transport

import (
	"context"
	"syscall"

	"github.com/chzyer/readline"
	"golang.org/x/sys/unix"
)

// watchEscDuringRun puts stdin into non-canonical poll mode and spawns a
// goroutine that scans for the ESC byte (0x1b). The first ESC cancels
// runCancel, modeling Claude Code / Codex behavior — operator interrupts
// the in-flight turn without killing the process.
//
// Safe to call ONLY between readline.Readline() invocations. chzyer/readline's
// IoLoop is dormant in that window (waiting on its kickChan) and the terminal
// is back in cooked mode, so no one is fighting for stdin. Calling this
// while readline is actively reading would race and steal its bytes.
//
// Other keys typed during the run are silently discarded — ECHO is off so
// they don't garble the agent's output. Use TCSETSF on restore to flush any
// stray bytes (multi-byte escape sequences, mistyped prompts) so they don't
// leak into the next readline prompt.
//
// Returns a cleanup func that stops the watcher and restores the terminal.
// On non-TTY stdin (piped input, non-interactive) returns a no-op cleanup.
func watchEscDuringRun(runCancel context.CancelFunc) (func(), error) {
	fd := readline.GetStdin()
	if !readline.IsTerminal(fd) {
		return func() {}, nil
	}
	return watchEscOnFd(fd, runCancel)
}

// watchEscOnFd is the testable core: takes an explicit fd so a unit test can
// drive it through a pty pair without depending on the process's stdin.
// Caller must ensure fd is a TTY (or any fd that supports termios ioctls).
func watchEscOnFd(fd int, runCancel context.CancelFunc) (func(), error) {
	orig, err := unix.IoctlGetTermios(fd, tcGet)
	if err != nil {
		return nil, err
	}
	t := *orig
	t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	// Keep ISIG: Ctrl+C still raises SIGINT, which installRunInterrupt
	// catches as a redundant cancel path (e.g. for `kill -INT pid`).
	t.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.IEXTEN
	// VMIN=0, VTIME=1 (100ms): read returns at least every 100ms, letting
	// the watcher poll its stop channel. Without this, a blocked read can't
	// be interrupted from Go.
	t.Cc[unix.VMIN] = 0
	t.Cc[unix.VTIME] = 1
	if err := unix.IoctlSetTermios(fd, tcSet, &t); err != nil {
		return nil, err
	}

	stopCh := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 32)
		for {
			select {
			case <-stopCh:
				return
			default:
			}
			n, err := syscall.Read(fd, buf)
			if err == syscall.EINTR {
				continue
			}
			if err != nil {
				return
			}
			for i := 0; i < n; i++ {
				if buf[i] == 0x1b {
					runCancel()
					return
				}
			}
		}
	}()

	return func() {
		close(stopCh)
		<-done
		// Flushing variant restores AND drops any half-typed escape sequence
		// or text the user banged on while the agent was running.
		_ = unix.IoctlSetTermios(fd, tcSetFlush, orig)
	}, nil
}
