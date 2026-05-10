//go:build linux

package transport

import "golang.org/x/sys/unix"

const (
	tcGet      = unix.TCGETS
	tcSet      = unix.TCSETS
	tcSetFlush = unix.TCSETSF
)
