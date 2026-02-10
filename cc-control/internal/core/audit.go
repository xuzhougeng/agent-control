package core

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

type AuditEvent struct {
	TsMS      int64          `json:"ts_ms"`
	Actor     string         `json:"actor"`
	ServerID  string         `json:"server_id,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	Kind      string         `json:"kind"`
	Meta      map[string]any `json:"meta,omitempty"`
}

type AuditLogger struct {
	mu   sync.Mutex
	file *os.File
}

func NewAuditLogger(path string) (*AuditLogger, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &AuditLogger{file: f}, nil
}

func (a *AuditLogger) Close() error {
	if a == nil || a.file == nil {
		return nil
	}
	return a.file.Close()
}

func (a *AuditLogger) Log(event AuditEvent) {
	if a == nil || a.file == nil {
		return
	}
	if event.TsMS == 0 {
		event.TsMS = time.Now().UnixMilli()
	}
	line, err := json.Marshal(event)
	if err != nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_, _ = a.file.Write(append(line, '\n'))
}
