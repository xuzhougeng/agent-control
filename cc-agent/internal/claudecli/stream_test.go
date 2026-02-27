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

func TestParseStreamJSON_CollectsOperations(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"system","subtype":"init","model":"claude-opus","permissionMode":"dontAsk","tools":["Bash","Read","Edit"]}`,
		`{"type":"assistant","message":{"content":[{"type":"thinking","text":"..."},{"type":"tool_use","name":"Bash","input":{"command":"echo hi"}},{"type":"text","text":"Hello"}]}}`,
		`{"type":"result","is_error":false,"result":"Hello","duration_ms":1234,"usage":{"input_tokens":10,"output_tokens":20},"total_cost_usd":0.01}`,
	}, "\n")
	res, err := ParseStreamJSON(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(res.Operations) < 3 {
		t.Fatalf("expected operations to be collected, got: %#v", res.Operations)
	}
	joined := strings.Join(res.Operations, "\n")
	if !strings.Contains(joined, "init: model=claude-opus") {
		t.Fatalf("missing init operation: %#v", res.Operations)
	}
	if !strings.Contains(joined, "tool_use: Bash") {
		t.Fatalf("missing tool_use operation: %#v", res.Operations)
	}
	if !strings.Contains(joined, "result: status=ok") {
		t.Fatalf("missing result operation: %#v", res.Operations)
	}
}
