//go:build windows

package transport

import "context"

// watchEscDuringRun is a no-op on Windows for now. Cancellation during a
// run still works via SIGINT (installRunInterrupt) — `kill -INT` from
// another shell — but the in-process ESC keypress is not yet wired up
// here because Windows console input requires a different code path
// (ReadConsoleInput / virtual-key polling) than the POSIX termios trick.
func watchEscDuringRun(_ context.CancelFunc) (func(), error) {
	return func() {}, nil
}
