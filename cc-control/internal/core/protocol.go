package core

import (
	"encoding/json"
	"time"
)

// Envelope is the common WS message format.
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
