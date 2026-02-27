package claudecli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

type StreamResult struct {
	Reply      string
	IsError    bool
	ErrText    string
	Operations []string
}

func ParseStreamJSON(r io.Reader) (StreamResult, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	var replyParts []string
	var resultText string
	var errLines []string
	var sawResult bool
	var isError bool
	var parsedAny bool
	var operations []string
	appendOp := func(op string) {
		op = strings.TrimSpace(op)
		if op == "" {
			return
		}
		if n := len(operations); n > 0 && operations[n-1] == op {
			return
		}
		operations = append(operations, op)
	}

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		parsedAny = true
		var ev streamEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "system":
			if op := formatInitOperation(ev); op != "" {
				appendOp(op)
			}
		case "assistant":
			for _, c := range ev.Message.Content {
				if c.Type == "text" {
					replyParts = append(replyParts, c.Text)
				}
			}
			for _, op := range extractAssistantOperations(ev.Message.Content) {
				appendOp(op)
			}
		case "result":
			sawResult = true
			isError = ev.IsError
			resultText = ev.Result
			if op := formatResultOperation(ev); op != "" {
				appendOp(op)
			}
			if len(ev.Errors) > 0 {
				errLines = append(errLines, ev.Errors...)
			}
			if len(ev.PermissionDenials) > 0 {
				errLines = append(errLines, parsePermissionDenials(ev.PermissionDenials)...)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return StreamResult{}, err
	}
	if !parsedAny {
		return StreamResult{}, errors.New("no stream-json output")
	}
	if sawResult && isError {
		errText := strings.TrimSpace(strings.Join(errLines, "\n"))
		if errText == "" {
			errText = strings.TrimSpace(resultText)
		}
		if errText == "" {
			errText = "unknown error"
		}
		return StreamResult{IsError: true, ErrText: errText, Operations: operations}, nil
	}
	reply := strings.TrimSpace(resultText)
	if reply == "" {
		reply = strings.TrimSpace(strings.Join(replyParts, ""))
	}
	if reply == "" {
		return StreamResult{}, errors.New("empty reply")
	}
	return StreamResult{Reply: reply, Operations: operations}, nil
}

type streamEvent struct {
	Type              string             `json:"type"`
	Subtype           string             `json:"subtype"`
	IsError           bool               `json:"is_error"`
	Result            string             `json:"result"`
	Errors            []string           `json:"errors"`
	PermissionDenials json.RawMessage    `json:"permission_denials"`
	Message           streamEventMessage `json:"message"`
	Tools             []string           `json:"tools"`
	Model             string             `json:"model"`
	PermissionMode    string             `json:"permissionMode"`
	DurationMS        int64              `json:"duration_ms"`
	NumTurns          int                `json:"num_turns"`
	TotalCostUSD      float64            `json:"total_cost_usd"`
	StopReason        string             `json:"stop_reason"`
	Usage             streamEventUsage   `json:"usage"`
}

type streamEventMessage struct {
	Content []streamEventContent `json:"content"`
}

type streamEventContent struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	Name     string          `json:"name"`
	ToolName string          `json:"tool_name"`
	Input    json.RawMessage `json:"input"`
	Content  json.RawMessage `json:"content"`
}

type streamEventUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func parsePermissionDenials(raw json.RawMessage) []string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	var strs []string
	if err := json.Unmarshal(raw, &strs); err == nil {
		return strs
	}
	var objs []map[string]any
	if err := json.Unmarshal(raw, &objs); err == nil {
		out := make([]string, 0, len(objs))
		for _, obj := range objs {
			if v, ok := obj["reason"].(string); ok && v != "" {
				out = append(out, v)
				continue
			}
			if v, ok := obj["message"].(string); ok && v != "" {
				out = append(out, v)
				continue
			}
			if b, err := json.Marshal(obj); err == nil {
				out = append(out, string(b))
			}
		}
		return out
	}
	return []string{string(raw)}
}

func formatInitOperation(ev streamEvent) string {
	if !strings.EqualFold(strings.TrimSpace(ev.Subtype), "init") {
		return ""
	}
	parts := make([]string, 0, 3)
	if model := strings.TrimSpace(ev.Model); model != "" {
		parts = append(parts, "model="+model)
	}
	if mode := strings.TrimSpace(ev.PermissionMode); mode != "" {
		parts = append(parts, "permission="+mode)
	}
	if len(ev.Tools) > 0 {
		parts = append(parts, "tools="+summarizeList(ev.Tools, 6))
	}
	if len(parts) == 0 {
		return "init"
	}
	return "init: " + strings.Join(parts, ", ")
}

func formatResultOperation(ev streamEvent) string {
	parts := []string{"status=ok"}
	if ev.IsError {
		parts[0] = "status=error"
	}
	if ev.DurationMS > 0 {
		parts = append(parts, fmt.Sprintf("duration=%dms", ev.DurationMS))
	}
	if ev.NumTurns > 0 {
		parts = append(parts, fmt.Sprintf("turns=%d", ev.NumTurns))
	}
	if ev.Usage.InputTokens > 0 {
		parts = append(parts, fmt.Sprintf("in=%d", ev.Usage.InputTokens))
	}
	if ev.Usage.OutputTokens > 0 {
		parts = append(parts, fmt.Sprintf("out=%d", ev.Usage.OutputTokens))
	}
	if ev.TotalCostUSD > 0 {
		parts = append(parts, fmt.Sprintf("cost=$%.6f", ev.TotalCostUSD))
	}
	if stop := strings.TrimSpace(ev.StopReason); stop != "" {
		parts = append(parts, "stop="+stop)
	}
	return "result: " + strings.Join(parts, ", ")
}

func extractAssistantOperations(content []streamEventContent) []string {
	ops := make([]string, 0, len(content))
	for _, c := range content {
		switch c.Type {
		case "tool_use":
			name := strings.TrimSpace(c.Name)
			if name == "" {
				name = strings.TrimSpace(c.ToolName)
			}
			if name == "" {
				name = "unknown"
			}
			op := "tool_use: " + name
			if input := summarizeJSONShape(c.Input); input != "" {
				op += " " + input
			}
			ops = append(ops, op)
		case "tool_result":
			if text := strings.TrimSpace(extractTextFromAny(c.Content)); text != "" {
				ops = append(ops, "tool_result: "+clipText(text, 120))
			} else if text := strings.TrimSpace(c.Text); text != "" {
				ops = append(ops, "tool_result: "+clipText(text, 120))
			} else {
				ops = append(ops, "tool_result")
			}
		case "thinking":
			ops = append(ops, "thinking")
		}
	}
	return ops
}

func summarizeList(items []string, limit int) string {
	clean := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		clean = append(clean, item)
	}
	if len(clean) == 0 {
		return ""
	}
	if limit <= 0 || len(clean) <= limit {
		return strings.Join(clean, ",")
	}
	head := strings.Join(clean[:limit], ",")
	return fmt.Sprintf("%s(+%d)", head, len(clean)-limit)
}

func summarizeJSONShape(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) == 0 {
			return "(input:empty)"
		}
		return "(input:" + summarizeList(keys, 5) + ")"
	}
	var arr []any
	if err := json.Unmarshal(raw, &arr); err == nil {
		return fmt.Sprintf("(input:array[%d])", len(arr))
	}
	return "(input:value)"
}

func extractTextFromAny(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	return extractTextValue(v)
}

func extractTextValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case map[string]any:
		if t, ok := x["text"].(string); ok && strings.TrimSpace(t) != "" {
			return t
		}
		for _, key := range []string{"result", "content", "message"} {
			if next, ok := x[key]; ok {
				if text := extractTextValue(next); strings.TrimSpace(text) != "" {
					return text
				}
			}
		}
	case []any:
		for _, item := range x {
			if text := extractTextValue(item); strings.TrimSpace(text) != "" {
				return text
			}
		}
	}
	return ""
}

func clipText(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
