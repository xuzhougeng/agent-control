//go:build !windows

package pty

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

type ptyLaunchConfig struct {
	setpgid bool
}

func newPTYLaunchConfig() ptyLaunchConfig {
	return ptyLaunchConfig{setpgid: true}
}

func configurePTYCmd(cmd *exec.Cmd, cfg ptyLaunchConfig) {
	if !cfg.setpgid {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: cfg.setpgid,
	}
}

func shouldRetryWithoutSetpgid(err error, cfg ptyLaunchConfig) bool {
	if err == nil || !cfg.setpgid {
		return false
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return errors.Is(pathErr.Err, syscall.EPERM)
	}
	return errors.Is(err, syscall.EPERM)
}

func disableSetpgid(cfg *ptyLaunchConfig) {
	if cfg == nil {
		return
	}
	cfg.setpgid = false
}

func interruptPTYTree(proc *os.Process, cfg ptyLaunchConfig) error {
	if !cfg.setpgid {
		return proc.Signal(syscall.SIGTERM)
	}
	return syscall.Kill(-proc.Pid, syscall.SIGTERM)
}

func killPTYTree(proc *os.Process, cfg ptyLaunchConfig) error {
	if !cfg.setpgid {
		return proc.Kill()
	}
	return syscall.Kill(-proc.Pid, syscall.SIGKILL)
}
