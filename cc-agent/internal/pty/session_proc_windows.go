//go:build windows

package pty

import (
	"os"
	"os/exec"
)

type ptyLaunchConfig struct{}

func newPTYLaunchConfig() ptyLaunchConfig {
	return ptyLaunchConfig{}
}

func configurePTYCmd(cmd *exec.Cmd, cfg ptyLaunchConfig) {
	_ = cmd
	_ = cfg
}

func shouldRetryWithoutSetpgid(err error, cfg ptyLaunchConfig) bool {
	_ = err
	_ = cfg
	return false
}

func disableSetpgid(cfg *ptyLaunchConfig) {
	_ = cfg
}

func interruptPTYTree(proc *os.Process, cfg ptyLaunchConfig) error {
	_ = cfg
	return proc.Kill()
}

func killPTYTree(proc *os.Process, cfg ptyLaunchConfig) error {
	_ = cfg
	return proc.Kill()
}
