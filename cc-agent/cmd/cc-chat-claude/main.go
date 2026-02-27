package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"cc-agent/internal/claudecli"
)

type Message struct {
	MessageID    string          `json:"message_id"`
	Content      string          `json:"content"`
	ContentParts []ContentPart   `json:"content_parts,omitempty"`
	SessionID    string          `json:"session_id,omitempty"`
	Meta         json.RawMessage `json:"meta,omitempty"`
}

type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type ContentPart struct {
	Type   string       `json:"type"`
	Text   string       `json:"text,omitempty"`
	Source *ImageSource `json:"source,omitempty"`
}

func main() {
	cfg := claudecli.LoadConfigFromEnv()
	sessionID := strings.TrimSpace(os.Getenv("CC_CHAT_SESSION_ID"))
	if sessionID == "" {
		sessionID = strings.TrimSpace(os.Getenv("CC_CLAUDE_SESSION_ID"))
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	writer := bufio.NewWriter(os.Stdout)
	defer writer.Flush()

	sessionReady := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if strings.TrimSpace(msg.Content) == "" && len(msg.ContentParts) == 0 {
			continue
		}

		reply, meta := handleMessage(cfg, sessionID, &sessionReady, msg.Content, msg.ContentParts)
		out := Message{MessageID: msg.MessageID, Content: reply, SessionID: sessionID, Meta: meta}
		data, _ := json.Marshal(out)
		writer.Write(data)
		writer.WriteString("\n")
		writer.Flush()
	}
}

func handleMessage(cfg claudecli.Config, sessionID string, sessionReady *bool, content string, parts []ContentPart) (string, json.RawMessage) {
	input := buildStreamInput(content, parts)
	useResume := *sessionReady
	var lastErr string
	var lastMeta json.RawMessage
	for i := 0; i < 2; i++ {
		reply, meta, errText, err := runClaude(cfg, sessionID, useResume, input)
		if err == nil && errText == "" {
			*sessionReady = true
			return reply, meta
		}
		if len(meta) > 0 {
			lastMeta = meta
		}
		if errText != "" {
			lastErr = errText
		}
		if err != nil && lastErr == "" {
			lastErr = err.Error()
		}

		if shouldRetryWithResume(errText) && !useResume {
			useResume = true
			continue
		}
		if shouldRetryWithSessionID(errText) && useResume {
			useResume = false
			continue
		}
		break
	}
	if lastErr == "" {
		lastErr = "claude execution failed"
	}
	return "Claude error: " + lastErr, lastMeta
}

func runClaude(cfg claudecli.Config, sessionID string, resume bool, input string) (string, json.RawMessage, string, error) {
	args := claudecli.BaseArgs(cfg)
	if sessionID != "" {
		if resume {
			args = append(args, "--resume", sessionID)
		} else {
			args = append(args, "--session-id", sessionID)
		}
	}

	ctx := context.Background()
	timeout := cfg.TimeoutMS
	if timeout <= 0 {
		timeout = 10 * 60 * 1000
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, cfg.Cmd, args...)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", nil, "", err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", nil, "", err
	}
	if err := cmd.Start(); err != nil {
		return "", nil, "", err
	}
	_, _ = stdin.Write([]byte(input))
	_ = stdin.Close()

	res, parseErr := claudecli.ParseStreamJSON(stdout)
	waitErr := cmd.Wait()
	meta := buildMeta(res.Operations)

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", nil, "", fmt.Errorf("timeout after %dms", timeout)
	}
	if parseErr != nil {
		return "", nil, "", parseErr
	}
	if res.IsError {
		return "", meta, res.ErrText, nil
	}
	if waitErr != nil {
		return "", meta, "", waitErr
	}
	return res.Reply, meta, "", nil
}

func buildStreamInput(content string, parts []ContentPart) string {
	contentItems := make([]map[string]any, 0, len(parts)+1)
	for _, p := range parts {
		switch strings.ToLower(strings.TrimSpace(p.Type)) {
		case "text":
			text := strings.TrimSpace(p.Text)
			if text == "" {
				continue
			}
			contentItems = append(contentItems, map[string]any{
				"type": "text",
				"text": text,
			})
		case "image":
			if p.Source == nil {
				continue
			}
			sourceType := strings.ToLower(strings.TrimSpace(p.Source.Type))
			mediaType := strings.ToLower(strings.TrimSpace(p.Source.MediaType))
			data := strings.TrimSpace(p.Source.Data)
			if sourceType != "base64" || mediaType == "" || data == "" {
				continue
			}
			contentItems = append(contentItems, map[string]any{
				"type": "image",
				"source": map[string]any{
					"type":       "base64",
					"media_type": mediaType,
					"data":       data,
				},
			})
		}
	}
	if len(contentItems) == 0 && strings.TrimSpace(content) != "" {
		contentItems = append(contentItems, map[string]any{
			"type": "text",
			"text": strings.TrimSpace(content),
		})
	}
	payload := map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": contentItems,
		},
	}
	data, _ := json.Marshal(payload)
	return string(data) + "\n"
}

func shouldRetryWithResume(errText string) bool {
	low := strings.ToLower(errText)
	return strings.Contains(low, "already in use")
}

func shouldRetryWithSessionID(errText string) bool {
	low := strings.ToLower(errText)
	return strings.Contains(low, "no conversation found")
}

func buildMeta(operations []string) json.RawMessage {
	clean := make([]string, 0, len(operations))
	for _, op := range operations {
		op = strings.TrimSpace(op)
		if op == "" {
			continue
		}
		if len(op) > 240 {
			op = op[:237] + "..."
		}
		clean = append(clean, op)
	}
	if len(clean) == 0 {
		return nil
	}
	if len(clean) > 20 {
		extra := len(clean) - 20
		clean = append(clean[:20], fmt.Sprintf("... %d more steps", extra))
	}
	payload := map[string]any{
		"source":     "claude-stream-json",
		"operations": clean,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return data
}
