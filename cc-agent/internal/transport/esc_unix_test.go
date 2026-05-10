//go:build !windows

package transport

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// openPty opens a pty pair via /dev/ptmx and returns the master os.File and
// the slave fd. Linux-only paths but adequate for our test target. The
// caller is responsible for closing both.
func openPty(t *testing.T) (master *os.File, slaveFd int) {
	t.Helper()
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatalf("open /dev/ptmx: %v", err)
	}
	// Unlock the slave so it's openable. TIOCSPTLCK takes a *int set to 0.
	zero := 0
	if err := unix.IoctlSetPointerInt(int(m.Fd()), unix.TIOCSPTLCK, zero); err != nil {
		m.Close()
		t.Fatalf("TIOCSPTLCK: %v", err)
	}
	num, err := unix.IoctlGetInt(int(m.Fd()), unix.TIOCGPTN)
	if err != nil {
		m.Close()
		t.Fatalf("TIOCGPTN: %v", err)
	}
	sname := fmt.Sprintf("/dev/pts/%d", num)
	s, err := os.OpenFile(sname, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		m.Close()
		t.Fatalf("open %s: %v", sname, err)
	}
	t.Cleanup(func() {
		_ = s.Close()
		_ = m.Close()
	})
	return m, int(s.Fd())
}

// TestWatchEscOnFd_EscCancels exercises the full path: termios ioctl, the
// goroutine reading from a real kernel pty, and the cancel fan-in.
func TestWatchEscOnFd_EscCancels(t *testing.T) {
	master, slaveFd := openPty(t)

	var cancelCount int32
	cancel := func() { atomic.AddInt32(&cancelCount, 1) }
	stop, err := watchEscOnFd(slaveFd, cancel)
	if err != nil {
		t.Fatalf("watchEscOnFd: %v", err)
	}
	defer stop()

	// Give the goroutine a moment to enter its read loop, then write ESC
	// to the master side. The watcher should see 0x1b on the slave.
	time.Sleep(50 * time.Millisecond)
	if _, err := master.Write([]byte{0x1b}); err != nil {
		t.Fatalf("write ESC: %v", err)
	}
	// VTIME=1 polls every 100ms, so cancel should fire well within 500ms.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&cancelCount) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if atomic.LoadInt32(&cancelCount) == 0 {
		t.Fatal("cancel was not called within 500ms after ESC")
	}
}

// TestWatchEscOnFd_OtherKeysDoNotCancel: typed bytes that aren't ESC must
// be silently consumed; cancel must not fire.
func TestWatchEscOnFd_OtherKeysDoNotCancel(t *testing.T) {
	master, slaveFd := openPty(t)

	var cancelCount int32
	cancel := func() { atomic.AddInt32(&cancelCount, 1) }
	stop, err := watchEscOnFd(slaveFd, cancel)
	if err != nil {
		t.Fatalf("watchEscOnFd: %v", err)
	}
	defer stop()

	time.Sleep(50 * time.Millisecond)
	if _, err := master.Write([]byte("abc123\n")); err != nil {
		t.Fatalf("write garbage: %v", err)
	}
	// Wait long enough for the watcher to process; cancel must remain 0.
	time.Sleep(300 * time.Millisecond)
	if atomic.LoadInt32(&cancelCount) != 0 {
		t.Errorf("cancel should NOT have been called; count=%d", atomic.LoadInt32(&cancelCount))
	}
}

// TestWatchEscOnFd_CleanupRestoresTermios: stop() must put the termios back
// to the originally-saved settings (VMIN/VTIME especially), so the next
// readline call doesn't inherit our 100ms-poll mode.
func TestWatchEscOnFd_CleanupRestoresTermios(t *testing.T) {
	_, slaveFd := openPty(t)
	before, err := unix.IoctlGetTermios(slaveFd, unix.TCGETS)
	if err != nil {
		t.Fatalf("TCGETS before: %v", err)
	}
	stop, err := watchEscOnFd(slaveFd, func() {})
	if err != nil {
		t.Fatalf("watchEscOnFd: %v", err)
	}
	stop()
	after, err := unix.IoctlGetTermios(slaveFd, unix.TCGETS)
	if err != nil {
		t.Fatalf("TCGETS after: %v", err)
	}
	if before.Lflag != after.Lflag {
		t.Errorf("Lflag not restored: before=%x after=%x", before.Lflag, after.Lflag)
	}
	if before.Cc[unix.VMIN] != after.Cc[unix.VMIN] || before.Cc[unix.VTIME] != after.Cc[unix.VTIME] {
		t.Errorf("VMIN/VTIME not restored: before=(%d,%d) after=(%d,%d)",
			before.Cc[unix.VMIN], before.Cc[unix.VTIME],
			after.Cc[unix.VMIN], after.Cc[unix.VTIME])
	}
}

// TestWatchEscOnFd_RapidStopBeforeAnyRead: stop() before any input still
// terminates the goroutine cleanly (no leak, no panic).
func TestWatchEscOnFd_RapidStopBeforeAnyRead(t *testing.T) {
	_, slaveFd := openPty(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop, err := watchEscOnFd(slaveFd, cancel)
	if err != nil {
		t.Fatalf("watchEscOnFd: %v", err)
	}
	// Don't wait — call stop immediately.
	stop()
	// ctx should NOT be canceled by stop() itself (only by ESC).
	select {
	case <-ctx.Done():
		t.Error("stop() should not cancel runCtx by itself")
	default:
	}
}
