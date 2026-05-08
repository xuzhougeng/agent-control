package agent

type EventKind string

const (
	EventUser       EventKind = "user"
	EventAssistant  EventKind = "assistant"
	EventToolCall   EventKind = "tool_call"
	EventToolResult EventKind = "tool_result"
	EventError      EventKind = "error"
)

// Event is emitted by the Agent during a Run, allowing callers to stream
// progress.
type Event struct {
	Kind      EventKind
	Text      string
	ToolName  string
	ToolID    string
	ToolInput map[string]any
	IsError   bool
}
