package claudecli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

type StreamResult struct {
	Reply   string
	IsError bool
	ErrText string
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
		case "assistant":
			for _, c := range ev.Message.Content {
				if c.Type == "text" {
					replyParts = append(replyParts, c.Text)
				}
			}
		case "result":
			sawResult = true
			isError = ev.IsError
			resultText = ev.Result
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
		return StreamResult{IsError: true, ErrText: errText}, nil
	}
	reply := strings.TrimSpace(resultText)
	if reply == "" {
		reply = strings.TrimSpace(strings.Join(replyParts, ""))
	}
	if reply == "" {
		return StreamResult{}, errors.New("empty reply")
	}
	return StreamResult{Reply: reply}, nil
}

type streamEvent struct {
	Type              string             `json:"type"`
	IsError           bool               `json:"is_error"`
	Result            string             `json:"result"`
	Errors            []string           `json:"errors"`
	PermissionDenials json.RawMessage    `json:"permission_denials"`
	Message           streamEventMessage `json:"message"`
}

type streamEventMessage struct {
	Content []streamEventContent `json:"content"`
}

type streamEventContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
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
