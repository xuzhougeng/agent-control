package config

import (
	"testing"
	"time"
)

func TestApprovalTimeoutDuration_Defaults(t *testing.T) {
	c := &Config{}
	if got := c.ApprovalTimeoutDuration(); got != 5*time.Minute {
		t.Errorf("empty timeout default = %v, want 5m", got)
	}
}

func TestApprovalTimeoutDuration_Parses(t *testing.T) {
	cases := map[string]time.Duration{
		"30s":  30 * time.Second,
		"5m":   5 * time.Minute,
		"1h":   1 * time.Hour,
		"500ms": 500 * time.Millisecond,
	}
	for in, want := range cases {
		c := &Config{ApprovalTimeout: in}
		if got := c.ApprovalTimeoutDuration(); got != want {
			t.Errorf("%q = %v, want %v", in, got, want)
		}
	}
}

func TestApprovalTimeoutDuration_FallsBackOnInvalid(t *testing.T) {
	for _, bad := range []string{"banana", "-5m", "0"} {
		c := &Config{ApprovalTimeout: bad}
		if got := c.ApprovalTimeoutDuration(); got != 5*time.Minute {
			t.Errorf("invalid %q = %v, want fallback 5m", bad, got)
		}
	}
}
