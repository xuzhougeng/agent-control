//go:build windows

package tools

import (
	"errors"
	"os/exec"
)

func RunAskpass(string) int { return 1 }

func startSudoAskpass([]byte) (func(*exec.Cmd), func(), error) {
	return nil, nil, errors.New("sudo askpass is not supported on windows")
}
