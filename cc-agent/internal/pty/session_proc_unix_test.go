//go:build !windows

package pty

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

func TestShouldRetryWithoutSetpgid(t *testing.T) {
	cfg := newPTYLaunchConfig()
	startErr := &os.PathError{Op: "fork/exec", Path: "/tmp/claude", Err: syscall.EPERM}
	if !shouldRetryWithoutSetpgid(startErr, cfg) {
		t.Fatal("expected retry for EPERM with setpgid enabled")
	}
}

func TestShouldRetryWithoutSetpgid_Disabled(t *testing.T) {
	cfg := ptyLaunchConfig{setpgid: false}
	startErr := &os.PathError{Op: "fork/exec", Path: "/tmp/claude", Err: syscall.EPERM}
	if shouldRetryWithoutSetpgid(startErr, cfg) {
		t.Fatal("did not expect retry when setpgid is already disabled")
	}
}

func TestShouldRetryWithoutSetpgid_NonEPERM(t *testing.T) {
	cfg := newPTYLaunchConfig()
	startErr := &os.PathError{Op: "fork/exec", Path: "/tmp/claude", Err: syscall.EACCES}
	if shouldRetryWithoutSetpgid(startErr, cfg) {
		t.Fatal("did not expect retry for non-EPERM start errors")
	}
	if shouldRetryWithoutSetpgid(errors.New("operation not permitted"), cfg) {
		t.Fatal("did not expect retry when EPERM cannot be proven")
	}
}

func TestDisableSetpgid(t *testing.T) {
	cfg := newPTYLaunchConfig()
	disableSetpgid(&cfg)
	if cfg.setpgid {
		t.Fatal("expected setpgid to be disabled")
	}
}
