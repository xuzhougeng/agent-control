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

func TestConfig_ControlHTTPURL_DerivedFromWS(t *testing.T) {
	cases := map[string]string{
		"ws://example.com:18180/ws/agent": "http://example.com:18180",
		"wss://example.com/ws/agent":      "https://example.com",
		"ws://localhost:18080":            "http://localhost:18080",
		"":                                "",
	}
	for in, want := range cases {
		c := &Config{ControlURL: in}
		c.applyDefaults()
		if c.ControlHTTPURL != want {
			t.Errorf("ControlURL=%q → ControlHTTPURL=%q, want %q", in, c.ControlHTTPURL, want)
		}
	}
}

func TestConfig_ControlHTTPURL_ExplicitWins(t *testing.T) {
	c := &Config{ControlURL: "ws://x:1/ws/agent", ControlHTTPURL: "http://override:9"}
	c.applyDefaults()
	if c.ControlHTTPURL != "http://override:9" {
		t.Errorf("override lost: %q", c.ControlHTTPURL)
	}
}
