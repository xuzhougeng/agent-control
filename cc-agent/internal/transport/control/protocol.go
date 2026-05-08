// Package control implements the cc-control WebSocket bridge.
//
// cc-agent only speaks chat mode — terminal/PTY sessions live in the cc-proxy
// module. The protocol envelope and payload shapes mirror cc-proxy's so a
// control plane sees one uniform agent surface.
package control

import (
	"encoding/json"
	"time"
)

// ProtocolVersion is the WebSocket protocol version cc-agent claims when
// registering. Must equal cc-control's ProtocolVersion (currently 1).
const ProtocolVersion = 1

type Envelope struct {
	Type       string          `json:"type"`
	ServerID   string          `json:"server_id,omitempty"`
	SessionID  string          `json:"session_id,omitempty"`
	InstanceID string          `json:"instance_id,omitempty"`
	Seq        uint64          `json:"seq,omitempty"`
	TsMS       int64           `json:"ts_ms,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
}

func newEnvelope(msgType, serverID, sessionID string) Envelope {
	return Envelope{
		Type: msgType, ServerID: serverID, SessionID: sessionID,
		TsMS: time.Now().UnixMilli(),
	}
}

// RegisterPayload announces this agent to the control plane.
// AllowRoots and ClaudePath are kept for protocol parity with cc-proxy; here
// AllowRoots is the agent's path containment list, and ClaudePath is reused
// to label the runtime ("cc-agent-builtin").
type RegisterPayload struct {
	ServerID        string   `json:"server_id"`
	Hostname        string   `json:"hostname"`
	Tags            []string `json:"tags"`
	OS              string   `json:"os"`
	Arch            string   `json:"arch"`
	AgentVersion    string   `json:"agent_version"`
	ProtocolVersion int      `json:"protocol_version"`
	AllowRoots      []string `json:"allow_roots"`
	ClaudePath      string   `json:"claude_path"`
}

// StartChatPayload requests creation of a chat session at the given cwd. We
// ignore worker_cmd / worker_args (cc-agent IS the worker).
type StartChatPayload struct {
	Cwd        string            `json:"cwd"`
	Env        map[string]string `json:"env"`
	WorkerCmd  string            `json:"worker_cmd,omitempty"`
	WorkerArgs []string          `json:"worker_args,omitempty"`
}

// ChatInPayload is one user message handed to the agent.
type ChatInPayload struct {
	MessageID    string            `json:"message_id"`
	Content      string            `json:"content"`
	ContentParts []ChatContentPart `json:"content_parts,omitempty"`
}

type ChatContentPart struct {
	Type   string           `json:"type"`
	Text   string           `json:"text,omitempty"`
	Source *ChatImageSource `json:"source,omitempty"`
}

type ChatImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// ChatOutPayload is one assistant reply. Meta carries tool-step summaries the
// UI displays under the bubble; standard shape is {"operations":[...]}.
type ChatOutPayload struct {
	MessageID string          `json:"message_id"`
	Content   string          `json:"content"`
	Meta      json.RawMessage `json:"meta,omitempty"`
}

// ChatOutMeta is the standard envelope cc-web understands. Set Progress=true
// for streamed intermediate updates (one bubble per tool step). Set
// Final=true on the closing message of the turn; cc-web swaps the progress
// bubbles for this one.
type ChatOutMeta struct {
	Source     string   `json:"source"`
	Operations []string `json:"operations,omitempty"`
	Progress   bool     `json:"progress,omitempty"`
	Final      bool     `json:"final,omitempty"`
	TsMS       int64    `json:"ts_ms,omitempty"`
}

// ChatExitPayload signals the chat instance has ended (errored or stopped).
type ChatExitPayload struct {
	ExitCode *int   `json:"exit_code,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// StopSessionPayload is the inbound stop request.
type StopSessionPayload struct {
	GraceMS     int    `json:"grace_ms"`
	KillAfterMS int    `json:"kill_after_ms"`
	Signal      string `json:"signal"`
}
