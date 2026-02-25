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
	MessageID string `json:"message_id"`
	Content   string `json:"content"`
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
		if strings.TrimSpace(msg.Content) == "" {
			continue
		}

		reply := handleMessage(cfg, sessionID, &sessionReady, msg.Content)
		out := Message{MessageID: msg.MessageID, Content: reply}
		data, _ := json.Marshal(out)
		writer.Write(data)
		writer.WriteString("\n")
		writer.Flush()
	}
}

func handleMessage(cfg claudecli.Config, sessionID string, sessionReady *bool, content string) string {
	input := buildStreamInput(content)
	useResume := *sessionReady
	var lastErr string
	for i := 0; i < 2; i++ {
		reply, errText, err := runClaude(cfg, sessionID, useResume, input)
		if err == nil && errText == "" {
			*sessionReady = true
			return reply
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
	return "Claude error: " + lastErr
}

func runClaude(cfg claudecli.Config, sessionID string, resume bool, input string) (string, string, error) {
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
		return "", "", err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", "", err
	}
	if err := cmd.Start(); err != nil {
		return "", "", err
	}
	_, _ = stdin.Write([]byte(input))
	_ = stdin.Close()

	res, parseErr := claudecli.ParseStreamJSON(stdout)
	waitErr := cmd.Wait()

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", "", fmt.Errorf("timeout after %dms", timeout)
	}
	if parseErr != nil {
		return "", "", parseErr
	}
	if res.IsError {
		return "", res.ErrText, nil
	}
	if waitErr != nil {
		return "", "", waitErr
	}
	return res.Reply, "", nil
}

func buildStreamInput(content string) string {
	payload := map[string]any{
		"type": "user",
		"message": map[string]any{
			"role": "user",
			"content": []map[string]string{
				{"type": "text", "text": content},
			},
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
