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

// OpenAIProvider speaks the OpenAI Chat Completions API. It is used for
// OpenAI itself, Azure-OpenAI compatible endpoints, DeepSeek, Qwen, vLLM,
// llama.cpp/server, and any other backend exposing /v1/chat/completions.
type OpenAIProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

type OpenAIOptions struct {
	APIKey  string
	BaseURL string // e.g. https://api.openai.com/v1 or https://api.deepseek.com/v1
	Timeout time.Duration
}

func NewOpenAI(opts OpenAIOptions) (*OpenAIProvider, error) {
	if opts.BaseURL == "" {
		return nil, errors.New("openai: base url required")
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	return &OpenAIProvider{
		apiKey:  opts.APIKey,
		baseURL: opts.BaseURL,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

func (p *OpenAIProvider) Name() string { return "openai" }

type openaiReq struct {
	Model    string          `json:"model"`
	Messages []openaiMessage `json:"messages"`
	Tools    []openaiTool    `json:"tools,omitempty"`
	MaxTok   int             `json:"max_tokens,omitempty"`
}

type openaiMessage struct {
	Role             string           `json:"role"`
	Content          string           `json:"content,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolCalls        []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
	Name             string           `json:"name,omitempty"`
}

type openaiToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function openaiFuncCall `json:"function"`
}

type openaiFuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openaiTool struct {
	Type     string         `json:"type"`
	Function openaiFuncSpec `json:"function"`
}

type openaiFuncSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type openaiResp struct {
	Choices []struct {
		Message      openaiMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *OpenAIProvider) Complete(ctx context.Context, req Request) (*Response, error) {
	msgs := toOpenAIMessages(req)
	body := openaiReq{
		Model:    req.Model,
		Messages: msgs,
		Tools:    toOpenAITools(req.Tools),
		MaxTok:   req.MaxTokens,
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	url := p.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("openai %d: %s", resp.StatusCode, string(raw))
	}
	var or openaiResp
	if err := json.Unmarshal(raw, &or); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if or.Error != nil {
		return nil, errors.New(or.Error.Message)
	}
	if len(or.Choices) == 0 {
		return nil, errors.New("openai: empty choices")
	}
	choice := or.Choices[0]

	msg := Message{
		Role:             RoleAssistant,
		Content:          choice.Message.Content,
		ReasoningContent: choice.Message.ReasoningContent,
	}
	for _, tc := range choice.Message.ToolCalls {
		var args map[string]any
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			args = map[string]any{"_raw": tc.Function.Arguments}
		}
		msg.ToolCalls = append(msg.ToolCalls, ToolCall{
			ID: tc.ID, Name: tc.Function.Name, Input: args,
		})
	}
	return &Response{
		Message:    msg,
		StopReason: mapOpenAIStop(choice.FinishReason, len(msg.ToolCalls) > 0),
		Usage: Usage{
			InputTokens:  or.Usage.PromptTokens,
			OutputTokens: or.Usage.CompletionTokens,
		},
	}, nil
}

func mapOpenAIStop(reason string, hasToolCalls bool) StopReason {
	if hasToolCalls || reason == "tool_calls" {
		return StopToolUse
	}
	switch reason {
	case "stop":
		return StopEnd
	case "length":
		return StopMaxTokens
	default:
		return StopOther
	}
}

func toOpenAIMessages(req Request) []openaiMessage {
	out := make([]openaiMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		out = append(out, openaiMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case RoleUser:
			out = append(out, openaiMessage{Role: "user", Content: m.Content})
		case RoleAssistant:
			om := openaiMessage{
				Role:             "assistant",
				Content:          m.Content,
				ReasoningContent: m.ReasoningContent,
			}
			for _, tc := range m.ToolCalls {
				args, _ := json.Marshal(tc.Input)
				om.ToolCalls = append(om.ToolCalls, openaiToolCall{
					ID: tc.ID, Type: "function",
					Function: openaiFuncCall{Name: tc.Name, Arguments: string(args)},
				})
			}
			out = append(out, om)
		case RoleTool:
			out = append(out, openaiMessage{
				Role:       "tool",
				Content:    m.ToolResult,
				ToolCallID: m.ToolUseID,
			})
		}
	}
	return out
}

func toOpenAITools(defs []ToolDef) []openaiTool {
	if len(defs) == 0 {
		return nil
	}
	out := make([]openaiTool, 0, len(defs))
	for _, d := range defs {
		out = append(out, openaiTool{
			Type: "function",
			Function: openaiFuncSpec{
				Name: d.Name, Description: d.Description, Parameters: d.InputSchema,
			},
		})
	}
	return out
}
