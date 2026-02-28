//go:build !windows

package echocli

import (
	"os"
	"os/exec"
	"syscall"
)

func configureWorkerCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

func interruptWorkerTree(proc *os.Process) error {
	return syscall.Kill(-proc.Pid, syscall.SIGINT)
}

func killWorkerTree(proc *os.Process) error {
	return syscall.Kill(-proc.Pid, syscall.SIGKILL)
}
