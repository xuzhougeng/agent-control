package claudecli

import (
	"strings"
	"testing"
)

func TestParseStreamJSON_SuccessPrefersResult(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"assistant","message":{"content":[{"type":"thinking","text":"..."},{"type":"text","text":"Hello "},{"type":"text","text":"XZG"}]}}`,
		`{"type":"result","is_error":false,"result":"Hello XZG"}`,
	}, "\n")
	res, err := ParseStreamJSON(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error: %s", res.ErrText)
	}
	if res.Reply != "Hello XZG" {
		t.Fatalf("unexpected reply: %q", res.Reply)
	}
}

func TestParseStreamJSON_Error(t *testing.T) {
	input := `{"type":"result","is_error":true,"errors":["Session ID already in use."]}`
	res, err := ParseStreamJSON(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected error result")
	}
	if !strings.Contains(res.ErrText, "already in use") {
		t.Fatalf("unexpected error text: %q", res.ErrText)
	}
}

func TestParseStreamJSON_PermissionDenials(t *testing.T) {
	input := `{"type":"result","is_error":true,"permission_denials":[{"reason":"tool_not_allowed"}]}`
	res, err := ParseStreamJSON(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected error result")
	}
	if !strings.Contains(res.ErrText, "tool_not_allowed") {
		t.Fatalf("unexpected error text: %q", res.ErrText)
	}
}
