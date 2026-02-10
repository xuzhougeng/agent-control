package agent

import (
	"encoding/json"
	"time"
)

type Envelope struct {
	Type      string          `json:"type"`
	ServerID  string          `json:"server_id,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	Seq       uint64          `json:"seq,omitempty"`
	TsMS      int64           `json:"ts_ms,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	DataB64   string          `json:"data_b64,omitempty"`
}

func NewEnvelope(msgType, serverID, sessionID string) Envelope {
	return Envelope{
		Type:      msgType,
		ServerID:  serverID,
		SessionID: sessionID,
		TsMS:      time.Now().UnixMilli(),
	}
}

type RegisterPayload struct {
	ServerID     string   `json:"server_id"`
	Hostname     string   `json:"hostname"`
	Tags         []string `json:"tags"`
	OS           string   `json:"os"`
	Arch         string   `json:"arch"`
	AgentVersion string   `json:"agent_version"`
	AllowRoots   []string `json:"allow_roots"`
	ClaudePath   string   `json:"claude_path"`
}

type StartSessionPayload struct {
	Cwd  string            `json:"cwd"`
	Cmd  []string          `json:"cmd"`
	Env  map[string]string `json:"env"`
	Cols uint16            `json:"cols"`
	Rows uint16            `json:"rows"`
}

type ResizePayload struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

type StopSessionPayload struct {
	GraceMS     int    `json:"grace_ms"`
	KillAfterMS int    `json:"kill_after_ms"`
	Signal      string `json:"signal"`
}

type PTYExitPayload struct {
	ExitCode *int   `json:"exit_code,omitempty"`
	Signal   string `json:"signal,omitempty"`
	Reason   string `json:"reason,omitempty"`
}
