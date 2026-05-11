package host

import (
	"os/exec"
	"runtime"
)

type ShellKind string

const (
	ShellPOSIX      ShellKind = "posix"
	ShellPowerShell ShellKind = "powershell"
	ShellCmd        ShellKind = "cmd"
)

type Shell struct {
	Kind    ShellKind
	Command string
}

type Context struct {
	GOOS   string
	GOARCH string
	Shell  Shell
}

func Current() Context {
	return Context{
		GOOS:   runtime.GOOS,
		GOARCH: runtime.GOARCH,
		Shell:  DetectShell(runtime.GOOS, exec.LookPath),
	}
}

func CurrentShell() Shell {
	return Current().Shell
}

func DetectShell(goos string, lookPath func(string) (string, error)) Shell {
	if goos == "windows" {
		for _, name := range []string{"pwsh.exe", "pwsh", "powershell.exe", "powershell"} {
			if _, err := lookPath(name); err == nil {
				return Shell{Kind: ShellPowerShell, Command: name}
			}
		}
		for _, name := range []string{"cmd.exe", "cmd"} {
			if _, err := lookPath(name); err == nil {
				return Shell{Kind: ShellCmd, Command: name}
			}
		}
		return Shell{Kind: ShellCmd, Command: "cmd.exe"}
	}
	return Shell{Kind: ShellPOSIX, Command: "sh"}
}

func (s Shell) CommandLine(command string) (string, []string) {
	name := s.Command
	if name == "" {
		name = s.defaultCommand()
	}
	switch s.Kind {
	case ShellPowerShell:
		return name, []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", command}
	case ShellCmd:
		return name, []string{"/C", command}
	default:
		return name, []string{"-c", command}
	}
}

func (s Shell) DisplayName() string {
	switch s.Kind {
	case ShellPowerShell:
		return "PowerShell"
	case ShellCmd:
		return "cmd.exe"
	default:
		return "POSIX shell"
	}
}

func (s Shell) InvocationDescription() string {
	name := s.Command
	if name == "" {
		name = s.defaultCommand()
	}
	switch s.Kind {
	case ShellPowerShell:
		return name + " -NoProfile -NonInteractive -ExecutionPolicy Bypass -Command <command>"
	case ShellCmd:
		return name + " /C <command>"
	default:
		return name + " -c <command>"
	}
}

func (s Shell) SyntaxHint() string {
	switch s.Kind {
	case ShellPowerShell:
		return "Use PowerShell syntax first: Get-ChildItem, Get-Content, Select-String, Get-Process, Get-Service, and $env:NAME for environment variables."
	case ShellCmd:
		return "Use cmd.exe syntax first: dir, type, findstr, tasklist, sc query, and %NAME% for environment variables."
	default:
		return "Use POSIX/Linux shell syntax first: ls, cat, grep, find, ps, df, systemctl when available, and $NAME for environment variables."
	}
}

func (s Shell) defaultCommand() string {
	switch s.Kind {
	case ShellPowerShell:
		return "powershell.exe"
	case ShellCmd:
		return "cmd.exe"
	default:
		return "sh"
	}
}
