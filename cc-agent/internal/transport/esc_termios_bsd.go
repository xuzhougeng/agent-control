//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package transport

import "golang.org/x/sys/unix"

const (
	tcGet      = unix.TIOCGETA
	tcSet      = unix.TIOCSETA
	tcSetFlush = unix.TIOCSETAF
)
