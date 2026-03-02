package claudecli

import "testing"

func TestSupportsSessionFlags(t *testing.T) {
	cases := []struct {
		name    string
		cmdPath string
		want    bool
	}{
		{name: "claude", cmdPath: "claude", want: true},
		{name: "claude code", cmdPath: "claude-code", want: true},
		{name: "claude absolute", cmdPath: "/usr/local/bin/claude", want: true},
		{name: "claude windows exe", cmdPath: `C:\tools\Claude.exe`, want: true},
		{name: "claude wrapper", cmdPath: "/opt/bin/claude-wrapper.sh", want: false},
		{name: "bash", cmdPath: "/usr/bin/bash", want: false},
		{name: "zsh", cmdPath: "zsh", want: false},
		{name: "empty", cmdPath: "   ", want: false},
	}

	for _, tc := range cases {
		if got := SupportsSessionFlags(tc.cmdPath); got != tc.want {
			t.Fatalf("%s: SupportsSessionFlags(%q) = %v, want %v", tc.name, tc.cmdPath, got, tc.want)
		}
	}
}
