package agent

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"runtime"
	"strings"
	"sync"
	"unicode"

	"cc-agent/internal/pty"
	"cc-agent/internal/security"
)

type Config struct {
	ServerID       string
	Hostname       string
	Tags           []string
	AllowRoots     []string
	ClaudePath     string
	EnvAllowKeys   map[string]struct{}
	EnvAllowPrefix string
}

type SessionManager struct {
	cfg Config

	sendMu   sync.RWMutex
	sendFunc func(msg Envelope) error

	mu       sync.RWMutex
	sessions map[string]*pty.Session
	pending  map[string]struct{}
}

func NewSessionManager(cfg Config) *SessionManager {
	return &SessionManager{
		cfg:      cfg,
		sessions: make(map[string]*pty.Session),
		pending:  make(map[string]struct{}),
	}
}

func (m *SessionManager) SetSendFunc(f func(msg Envelope) error) {
	m.sendMu.Lock()
	defer m.sendMu.Unlock()
	m.sendFunc = f
}

func (m *SessionManager) send(msg Envelope) error {
	m.sendMu.RLock()
	f := m.sendFunc
	m.sendMu.RUnlock()
	if f == nil {
		return errors.New("send function not set")
	}
	return f(msg)
}

func (m *SessionManager) RegisterPayload() RegisterPayload {
	return RegisterPayload{
		ServerID:     m.cfg.ServerID,
		Hostname:     m.cfg.Hostname,
		Tags:         append([]string(nil), m.cfg.Tags...),
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		AgentVersion: "0.1.0",
		AllowRoots:   append([]string(nil), m.cfg.AllowRoots...),
		ClaudePath:   m.cfg.ClaudePath,
	}
}

func (m *SessionManager) Handle(msg Envelope) error {
	switch msg.Type {
	case "start_session":
		var req StartSessionPayload
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return err
		}
		return m.startSession(msg.SessionID, req)
	case "pty_in":
		return m.writeSession(msg.SessionID, msg.DataB64)
	case "resize":
		var req ResizePayload
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return err
		}
		return m.resizeSession(msg.SessionID, req.Cols, req.Rows)
	case "stop_session":
		var req StopSessionPayload
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return err
		}
		return m.stopSession(msg.SessionID, req.GraceMS, req.KillAfterMS)
	case "heartbeat":
		return nil
	default:
		return nil
	}
}

func (m *SessionManager) startSession(sessionID string, req StartSessionPayload) error {
	if sessionID == "" {
		return errors.New("missing session_id")
	}

	// Atomically check sessions + pending, then mark pending
	m.mu.Lock()
	if _, ok := m.sessions[sessionID]; ok {
		m.mu.Unlock()
		return errors.New("session already exists")
	}
	if _, ok := m.pending[sessionID]; ok {
		m.mu.Unlock()
		return errors.New("session already starting")
	}
	m.pending[sessionID] = struct{}{}
	m.mu.Unlock()

	// Ensure pending is cleaned up regardless of outcome
	defer func() {
		m.mu.Lock()
		delete(m.pending, sessionID)
		m.mu.Unlock()
	}()

	if err := security.ValidateCWD(req.Cwd, m.cfg.AllowRoots); err != nil {
		m.sendError(sessionID, "reject_cwd:"+err.Error())
		return err
	}

	resumeID := strings.TrimSpace(req.ResumeID)
	if resumeID != "" {
		if len(resumeID) > 128 {
			err := errors.New("resume_id too long")
			m.sendError(sessionID, "reject_resume_id:too_long")
			return err
		}
		for _, r := range resumeID {
			if unicode.IsSpace(r) {
				err := errors.New("resume_id contains whitespace")
				m.sendError(sessionID, "reject_resume_id:contains_whitespace")
				return err
			}
		}
	}

	args := make([]string, 0, 2)
	if resumeID != "" {
		args = append(args, "--resume", resumeID)
	}

	env := security.FilterEnv(req.Env, m.cfg.EnvAllowKeys, m.cfg.EnvAllowPrefix)
	sess, err := pty.Start(sessionID, req.Cwd, m.cfg.ClaudePath, args, env, req.Cols, req.Rows)
	if err != nil {
		m.sendError(sessionID, "start_failed:"+err.Error())
		return err
	}

	m.mu.Lock()
	m.sessions[sessionID] = sess
	m.mu.Unlock()

	go sess.ReadLoop(func(seq uint64, chunk []byte) {
		msg := NewEnvelope("pty_out", m.cfg.ServerID, sessionID)
		msg.Seq = seq
		msg.DataB64 = base64.StdEncoding.EncodeToString(chunk)
		if err := m.send(msg); err != nil {
			log.Printf("send pty_out failed session=%s: %v", sessionID, err)
		}
	}, func(code *int, signal, reason string) {
		m.mu.Lock()
		delete(m.sessions, sessionID)
		m.mu.Unlock()
		payload, _ := json.Marshal(PTYExitPayload{
			ExitCode: code,
			Signal:   signal,
			Reason:   reason,
		})
		msg := NewEnvelope("pty_exit", m.cfg.ServerID, sessionID)
		msg.Data = payload
		_ = m.send(msg)
	})
	return nil
}

func (m *SessionManager) writeSession(sessionID, dataB64 string) error {
	raw, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return err
	}
	m.mu.RLock()
	sess := m.sessions[sessionID]
	m.mu.RUnlock()
	if sess == nil {
		return errors.New("session not found")
	}
	return sess.Write(raw)
}

func (m *SessionManager) resizeSession(sessionID string, cols, rows uint16) error {
	m.mu.RLock()
	sess := m.sessions[sessionID]
	m.mu.RUnlock()
	if sess == nil {
		return errors.New("session not found")
	}
	return sess.Resize(cols, rows)
}

func (m *SessionManager) stopSession(sessionID string, graceMS, killAfterMS int) error {
	m.mu.RLock()
	sess := m.sessions[sessionID]
	m.mu.RUnlock()
	if sess == nil {
		return errors.New("session not found")
	}
	go sess.Stop(graceMS, killAfterMS)
	return nil
}

func (m *SessionManager) sendError(sessionID, message string) {
	payload, _ := json.Marshal(map[string]string{
		"message": message,
	})
	msg := NewEnvelope("error", m.cfg.ServerID, sessionID)
	msg.Data = payload
	_ = m.send(msg)
}
