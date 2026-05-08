// Package llm defines a provider-agnostic chat completion abstraction with
// tool-use support. Concrete providers (Anthropic, OpenAI-compatible) live in
// sibling files.
package llm

import "context"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is one turn in a conversation. A message can carry text content,
// tool calls (assistant-issued), or a tool result (role=tool, ToolUseID set).
//
// ReasoningContent carries the chain-of-thought emitted by reasoning models
// (DeepSeek v4-pro, Qwen thinking variants, …). Some providers require it to
// be echoed back verbatim in the next turn; the agent persists and replays it.
type Message struct {
	Role             Role
	Content          string
	ReasoningContent string
	ToolCalls        []ToolCall
	ToolUseID        string
	ToolResult       string
	ToolError        bool
}

// ToolCall is a model-issued request to invoke a tool.
type ToolCall struct {
	ID    string
	Name  string
	Input map[string]any
}

// ToolDef describes a tool available to the model.
type ToolDef struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type StopReason string

const (
	StopEnd       StopReason = "end_turn"
	StopToolUse   StopReason = "tool_use"
	StopMaxTokens StopReason = "max_tokens"
	StopOther     StopReason = "other"
)

// Request is a single completion request.
type Request struct {
	System    string
	Messages  []Message
	Tools     []ToolDef
	Model     string
	MaxTokens int
}

// Response is the model output for one Request.
type Response struct {
	Message    Message
	StopReason StopReason
	Usage      Usage
}

type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Provider abstracts an LLM backend.
type Provider interface {
	Name() string
	Complete(ctx context.Context, req Request) (*Response, error)
}
