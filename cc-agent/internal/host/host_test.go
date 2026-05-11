package host

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestDetectShell_WindowsPrefersPowerShell(t *testing.T) {
	got := DetectShell("windows", fakeLookPath(map[string]bool{
		"powershell.exe": true,
		"cmd.exe":        true,
	}))
	if got.Kind != ShellPowerShell {
		t.Fatalf("kind = %q, want %q", got.Kind, ShellPowerShell)
	}
	if got.Command != "powershell.exe" {
		t.Fatalf("command = %q, want powershell.exe", got.Command)
	}
	name, args := got.CommandLine("Get-Process")
	if name != "powershell.exe" {
		t.Fatalf("command line executable = %q", name)
	}
	wantArgs := []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", "Get-Process"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

func TestDetectShell_WindowsFallsBackToCmd(t *testing.T) {
	got := DetectShell("windows", fakeLookPath(map[string]bool{
		"cmd.exe": true,
	}))
	if got.Kind != ShellCmd {
		t.Fatalf("kind = %q, want %q", got.Kind, ShellCmd)
	}
	if got.Command != "cmd.exe" {
		t.Fatalf("command = %q, want cmd.exe", got.Command)
	}
	_, args := got.CommandLine("dir")
	if !reflect.DeepEqual(args, []string{"/C", "dir"}) {
		t.Fatalf("args = %#v", args)
	}
}

func TestDetectShell_UnixUsesPOSIXShell(t *testing.T) {
	got := DetectShell("linux", fakeLookPath(nil))
	if got.Kind != ShellPOSIX {
		t.Fatalf("kind = %q, want %q", got.Kind, ShellPOSIX)
	}
	if got.Command != "sh" {
		t.Fatalf("command = %q, want sh", got.Command)
	}
	if !strings.Contains(got.SyntaxHint(), "POSIX") {
		t.Fatalf("syntax hint should mention POSIX, got %q", got.SyntaxHint())
	}
}

func fakeLookPath(found map[string]bool) func(string) (string, error) {
	return func(name string) (string, error) {
		if found[name] {
			return name, nil
		}
		return "", errors.New("not found")
	}
}
