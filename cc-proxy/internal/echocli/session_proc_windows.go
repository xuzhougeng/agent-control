//go:build windows

package echocli

import (
	"os"
	"os/exec"
)

func configureWorkerCmd(cmd *exec.Cmd) {}

func interruptWorkerTree(proc *os.Process) error {
	return proc.Signal(os.Interrupt)
}

func killWorkerTree(proc *os.Process) error {
	return proc.Kill()
}
