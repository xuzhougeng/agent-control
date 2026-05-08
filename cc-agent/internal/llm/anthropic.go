package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	anthropicURL     = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
)

type AnthropicProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

type AnthropicOptions struct {
	APIKey  string
	BaseURL string
	Timeout time.Duration
}

func NewAnthropic(opts AnthropicOptions) (*AnthropicProvider, error) {
	if opts.APIKey == "" {
		return nil, errors.New("anthropic: api key required")
	}
	url := opts.BaseURL
	if url == "" {
		url = anthropicURL
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	return &AnthropicProvider{
		apiKey:  opts.APIKey,
		baseURL: url,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

func (p *AnthropicProvider) Name() string { return "anthropic" }

type anthropicReq struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicContent struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   string         `json:"content,omitempty"`
	IsError   bool           `json:"is_error,omitempty"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

type anthropicResp struct {
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Role       string             `json:"role"`
	Content    []anthropicContent `json:"content"`
	Model      string             `json:"model"`
	StopReason string             `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *AnthropicProvider) Complete(ctx context.Context, req Request) (*Response, error) {
	body := anthropicReq{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		System:    req.System,
		Messages:  toAnthropicMessages(req.Messages),
		Tools:     toAnthropicTools(req.Tools),
	}
	if body.MaxTokens == 0 {
		body.MaxTokens = 4096
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("anthropic %d: %s", resp.StatusCode, string(raw))
	}
	var ar anthropicResp
	if err := json.Unmarshal(raw, &ar); err != nil {
		return nil, fmt.Errorf("decode: %w (body: %s)", err, string(raw))
	}
	if ar.Error != nil {
		return nil, fmt.Errorf("anthropic error: %s: %s", ar.Error.Type, ar.Error.Message)
	}

	msg := Message{Role: RoleAssistant}
	for _, c := range ar.Content {
		switch c.Type {
		case "text":
			if msg.Content != "" {
				msg.Content += "\n"
			}
			msg.Content += c.Text
		case "tool_use":
			msg.ToolCalls = append(msg.ToolCalls, ToolCall{
				ID: c.ID, Name: c.Name, Input: c.Input,
			})
		}
	}

	return &Response{
		Message:    msg,
		StopReason: mapAnthropicStop(ar.StopReason),
		Usage: Usage{
			InputTokens:  ar.Usage.InputTokens,
			OutputTokens: ar.Usage.OutputTokens,
		},
	}, nil
}

func mapAnthropicStop(s string) StopReason {
	switch s {
	case "end_turn":
		return StopEnd
	case "tool_use":
		return StopToolUse
	case "max_tokens":
		return StopMaxTokens
	default:
		return StopOther
	}
}

func toAnthropicTools(defs []ToolDef) []anthropicTool {
	if len(defs) == 0 {
		return nil
	}
	out := make([]anthropicTool, 0, len(defs))
	for _, d := range defs {
		out = append(out, anthropicTool{
			Name:        d.Name,
			Description: d.Description,
			InputSchema: d.InputSchema,
		})
	}
	return out
}

func toAnthropicMessages(msgs []Message) []anthropicMessage {
	out := make([]anthropicMessage, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case RoleUser:
			am := anthropicMessage{Role: "user"}
			if m.Content != "" {
				am.Content = append(am.Content, anthropicContent{Type: "text", Text: m.Content})
			}
			out = append(out, am)
		case RoleAssistant:
			am := anthropicMessage{Role: "assistant"}
			if m.Content != "" {
				am.Content = append(am.Content, anthropicContent{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				am.Content = append(am.Content, anthropicContent{
					Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: tc.Input,
				})
			}
			out = append(out, am)
		case RoleTool:
			out = append(out, anthropicMessage{
				Role: "user",
				Content: []anthropicContent{{
					Type:      "tool_result",
					ToolUseID: m.ToolUseID,
					Content:   m.ToolResult,
					IsError:   m.ToolError,
				}},
			})
		}
	}
	return out
}
