// Package memory persists session/message state. Two implementations exist:
// an in-memory map (default) and a SQLite-backed store for ops deployments.
package memory

import (
	"context"
	"encoding/json"
	"sync"

	"cc-agent/internal/llm"
)

// Message is the persistence-friendly form of llm.Message: tool calls become
// JSON-encoded blobs so the schema stays flat.
type Message struct {
	ID               int64
	Role             string
	Content          string
	ReasoningContent string
	ToolCalls        string // json: []{id,name,input}
	ToolUseID        string
	ToolResult       string
	ToolError        bool
	At               int64
}

// FromLLM converts an llm.Message into a Message ready to persist.
func FromLLM(m llm.Message) Message {
	out := Message{
		Role:             string(m.Role),
		Content:          m.Content,
		ReasoningContent: m.ReasoningContent,
		ToolUseID:        m.ToolUseID,
		ToolResult:       m.ToolResult,
		ToolError:        m.ToolError,
	}
	if len(m.ToolCalls) > 0 {
		b, _ := json.Marshal(m.ToolCalls)
		out.ToolCalls = string(b)
	}
	return out
}

// ToLLM rehydrates a Message into an llm.Message for sending to a provider.
func (m Message) ToLLM() llm.Message {
	out := llm.Message{
		Role:             llm.Role(m.Role),
		Content:          m.Content,
		ReasoningContent: m.ReasoningContent,
		ToolUseID:        m.ToolUseID,
		ToolResult:       m.ToolResult,
		ToolError:        m.ToolError,
	}
	if m.ToolCalls != "" {
		_ = json.Unmarshal([]byte(m.ToolCalls), &out.ToolCalls)
	}
	return out
}

// Store is the persistence interface; in-memory and SQLite both satisfy it.
type Store interface {
	EnsureSession(ctx context.Context, id string) error
	AppendMessage(ctx context.Context, sessionID string, m Message) error
	LoadMessages(ctx context.Context, sessionID string) ([]Message, error)
	Sessions(ctx context.Context) ([]string, error)
	// DropAfter removes every message in sessionID with ID strictly greater
	// than afterID. Pass 0 to drop the whole session's messages. Returns the
	// number of rows removed. Used by Agent.Rewind to support :rewind.
	DropAfter(ctx context.Context, sessionID string, afterID int64) (int, error)
	Close() error
}

// InMemory is a goroutine-safe Store useful for tests and ephemeral CLI runs.
type InMemory struct {
	mu       sync.RWMutex
	sessions map[string][]Message
	nextID   int64
}

func NewInMemory() *InMemory {
	return &InMemory{sessions: map[string][]Message{}}
}

func (m *InMemory) EnsureSession(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[id]; !ok {
		m.sessions[id] = nil
	}
	return nil
}

func (m *InMemory) AppendMessage(_ context.Context, id string, msg Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	msg.ID = m.nextID
	m.sessions[id] = append(m.sessions[id], msg)
	return nil
}

func (m *InMemory) LoadMessages(_ context.Context, id string) ([]Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	src := m.sessions[id]
	out := make([]Message, len(src))
	copy(out, src)
	return out, nil
}

func (m *InMemory) Sessions(_ context.Context) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.sessions))
	for k := range m.sessions {
		out = append(out, k)
	}
	return out, nil
}

func (m *InMemory) DropAfter(_ context.Context, id string, afterID int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	src := m.sessions[id]
	keep := src[:0]
	dropped := 0
	for _, msg := range src {
		if msg.ID > afterID {
			dropped++
			continue
		}
		keep = append(keep, msg)
	}
	m.sessions[id] = keep
	return dropped, nil
}

func (m *InMemory) Close() error { return nil }
