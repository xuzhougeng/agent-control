package agent

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"cc-agent/internal/echocli"
	"cc-agent/internal/pty"
	"cc-agent/internal/security"
)

const ptyUnsupportedOnWindowsMessage = "PTY is not supported on Windows yet; use session_type=chat"

var runtimeGOOS = runtime.GOOS

type Config struct {
	ServerID       string
	Hostname       string
	Tags           []string
	AllowRoots     []string
	ClaudePath     string
	ChatWorkerCmd  string
	ChatWorkerArgs []string
	EnvAllowKeys   map[string]struct{}
	EnvAllowPrefix string
}

type SessionManager struct {
	cfg Config

	sendMu   sync.RWMutex
	sendFunc func(msg Envelope) error

	mu           sync.RWMutex
	sessions     map[string]*pty.Session
	chatSessions map[string]*echocli.Session
	pending      map[string]struct{}
}

func NewSessionManager(cfg Config) *SessionManager {
	return &SessionManager{
		cfg:          cfg,
		sessions:     make(map[string]*pty.Session),
		chatSessions: make(map[string]*echocli.Session),
		pending:      make(map[string]struct{}),
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
		return m.startSession(msg.SessionID, msg.InstanceID, req)
	case "pty_in":
		return m.writeSession(msg.InstanceID, msg.DataB64)
	case "resize":
		var req ResizePayload
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return err
		}
		return m.resizeSession(msg.InstanceID, req.Cols, req.Rows)
	case "stop_session":
		var req StopSessionPayload
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return err
		}
		return m.stopSession(msg.InstanceID, req.GraceMS, req.KillAfterMS)
	case "start_chat":
		var req StartChatPayload
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return err
		}
		return m.startChat(msg.SessionID, msg.InstanceID, req)
	case "chat_in":
		var req ChatInPayload
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return err
		}
		return m.writeChatSession(msg.InstanceID, req)
	case "heartbeat":
		return nil
	default:
		return nil
	}
}

func (m *SessionManager) startSession(sessionID, instanceID string, req StartSessionPayload) error {
	if sessionID == "" || instanceID == "" {
		return errors.New("missing session_id or instance_id")
	}

	// Atomically check sessions + pending, then mark pending
	m.mu.Lock()
	if _, ok := m.sessions[instanceID]; ok {
		m.mu.Unlock()
		return errors.New("session already exists")
	}
	if _, ok := m.chatSessions[instanceID]; ok {
		m.mu.Unlock()
		return errors.New("session already exists")
	}
	if _, ok := m.pending[instanceID]; ok {
		m.mu.Unlock()
		return errors.New("session already starting")
	}
	m.pending[instanceID] = struct{}{}
	m.mu.Unlock()

	// Ensure pending is cleaned up regardless of outcome
	defer func() {
		m.mu.Lock()
		delete(m.pending, instanceID)
		m.mu.Unlock()
	}()

	if err := security.ValidateCWD(req.Cwd, m.cfg.AllowRoots); err != nil {
		m.sendError(sessionID, instanceID, "reject_cwd:"+err.Error())
		return err
	}

	cmdPath := strings.TrimSpace(m.cfg.ClaudePath)
	args := []string{"--session-id", sessionID}
	if len(req.Cmd) > 0 {
		cmdPath = strings.TrimSpace(req.Cmd[0])
		if len(req.Cmd) > 1 {
			args = append([]string(nil), req.Cmd[1:]...)
		} else {
			args = nil
		}
	}
	if cmdPath == "" {
		cmdPath = "claude-code"
	}
	args = normalizeClaudeSessionArgs(sessionID, args)

	env := security.FilterEnv(req.Env, m.cfg.EnvAllowKeys, m.cfg.EnvAllowPrefix)
	if strings.EqualFold(runtimeGOOS, "windows") {
		err := errors.New(ptyUnsupportedOnWindowsMessage)
		m.sendError(sessionID, instanceID, "start_failed:"+err.Error())
		return err
	}
	sess, err := pty.Start(sessionID, req.Cwd, cmdPath, args, env, req.Cols, req.Rows)
	if err != nil {
		m.sendError(sessionID, instanceID, "start_failed:"+err.Error())
		return err
	}

	m.mu.Lock()
	m.sessions[instanceID] = sess
	m.mu.Unlock()

	go sess.ReadLoop(func(seq uint64, chunk []byte) {
		msg := NewEnvelope("pty_out", m.cfg.ServerID, sessionID)
		msg.InstanceID = instanceID
		msg.Seq = seq
		msg.DataB64 = base64.StdEncoding.EncodeToString(chunk)
		if err := m.send(msg); err != nil {
			log.Printf("send pty_out failed session=%s: %v", sessionID, err)
		}
	}, func(code *int, signal, reason string) {
		m.mu.Lock()
		delete(m.sessions, instanceID)
		m.mu.Unlock()
		payload, _ := json.Marshal(PTYExitPayload{
			ExitCode: code,
			Signal:   signal,
			Reason:   reason,
		})
		msg := NewEnvelope("pty_exit", m.cfg.ServerID, sessionID)
		msg.InstanceID = instanceID
		msg.Data = payload
		_ = m.send(msg)
	})
	return nil
}

func normalizeClaudeSessionArgs(sessionID string, args []string) []string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || len(args) < 2 {
		return args
	}
	for i := 0; i+1 < len(args); i++ {
		flag := strings.TrimSpace(args[i])
		value := strings.TrimSpace(args[i+1])
		if value != sessionID {
			continue
		}
		exists := claudeSessionEnvExists(sessionID)
		switch flag {
		case "--session-id":
			if exists {
				out := append([]string(nil), args...)
				out[i] = "--resume"
				return out
			}
		}
	}
	return args
}

func claudeSessionEnvExists(sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return false
	}
	path := filepath.Join(home, ".claude", "session-env", sessionID)
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func (m *SessionManager) writeSession(instanceID, dataB64 string) error {
	raw, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return err
	}
	m.mu.RLock()
	sess := m.sessions[instanceID]
	m.mu.RUnlock()
	if sess == nil {
		return errors.New("session not found")
	}
	return sess.Write(raw)
}

func (m *SessionManager) resizeSession(instanceID string, cols, rows uint16) error {
	m.mu.RLock()
	sess := m.sessions[instanceID]
	m.mu.RUnlock()
	if sess == nil {
		return errors.New("session not found")
	}
	return sess.Resize(cols, rows)
}

func (m *SessionManager) stopSession(instanceID string, graceMS, killAfterMS int) error {
	m.mu.RLock()
	sess := m.sessions[instanceID]
	csess := m.chatSessions[instanceID]
	m.mu.RUnlock()
	if sess != nil {
		go sess.Stop(graceMS, killAfterMS)
		return nil
	}
	if csess != nil {
		go csess.Stop(graceMS, killAfterMS)
		return nil
	}
	return errors.New("session not found")
}

func (m *SessionManager) startChat(sessionID, instanceID string, req StartChatPayload) error {
	if sessionID == "" || instanceID == "" {
		return errors.New("missing session_id or instance_id")
	}

	m.mu.Lock()
	if _, ok := m.chatSessions[instanceID]; ok {
		m.mu.Unlock()
		return errors.New("chat session already exists")
	}
	if _, ok := m.sessions[instanceID]; ok {
		m.mu.Unlock()
		return errors.New("session already exists")
	}
	if _, ok := m.pending[instanceID]; ok {
		m.mu.Unlock()
		return errors.New("session already starting")
	}
	m.pending[instanceID] = struct{}{}
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.pending, instanceID)
		m.mu.Unlock()
	}()

	if err := security.ValidateCWD(req.Cwd, m.cfg.AllowRoots); err != nil {
		m.sendError(sessionID, instanceID, "reject_cwd:"+err.Error())
		return err
	}

	workerCmd := req.WorkerCmd
	workerArgs := req.WorkerArgs
	if workerCmd == "" {
		workerCmd = m.cfg.ChatWorkerCmd
		workerArgs = m.cfg.ChatWorkerArgs
	}
	if workerCmd == "" {
		m.sendError(sessionID, instanceID, "no chat worker configured")
		return errors.New("no chat worker configured")
	}

	env := security.FilterEnv(req.Env, m.cfg.EnvAllowKeys, m.cfg.EnvAllowPrefix)
	if env == nil {
		env = make(map[string]string)
	}
	env["CC_CHAT_SESSION_ID"] = sessionID
	env["CC_CHAT_INSTANCE_ID"] = instanceID
	env["CC_AGENT_SERVER_ID"] = m.cfg.ServerID
	env["CC_AGENT_HOSTNAME"] = m.cfg.Hostname
	env["CC_AGENT_OS"] = runtimeGOOS
	env["CC_AGENT_ARCH"] = runtime.GOARCH
	env["CC_AGENT_SESSION_CWD"] = req.Cwd
	if len(m.cfg.AllowRoots) > 0 {
		env["CC_AGENT_ALLOW_ROOTS"] = strings.Join(m.cfg.AllowRoots, ",")
	}
	if len(m.cfg.Tags) > 0 {
		env["CC_AGENT_TAGS"] = strings.Join(m.cfg.Tags, ",")
	}
	csess, err := echocli.Start(sessionID, req.Cwd, workerCmd, workerArgs, env)
	if err != nil {
		m.sendError(sessionID, instanceID, "chat_start_failed:"+err.Error())
		return err
	}

	csess.SetCallbacks(func(msg echocli.Message) {
		payload, _ := json.Marshal(ChatOutPayload{
			MessageID: msg.MessageID,
			Content:   msg.Content,
			Meta:      msg.Meta,
		})
		env := NewEnvelope("chat_out", m.cfg.ServerID, sessionID)
		env.InstanceID = instanceID
		env.Data = payload
		if err := m.send(env); err != nil {
			log.Printf("send chat_out failed session=%s: %v", sessionID, err)
		}
	}, func(code *int, reason string) {
		m.mu.Lock()
		delete(m.chatSessions, instanceID)
		m.mu.Unlock()
		payload, _ := json.Marshal(ChatExitPayload{
			ExitCode: code,
			Reason:   reason,
		})
		env := NewEnvelope("chat_exit", m.cfg.ServerID, sessionID)
		env.InstanceID = instanceID
		env.Data = payload
		_ = m.send(env)
	})

	m.mu.Lock()
	m.chatSessions[instanceID] = csess
	m.mu.Unlock()
	return nil
}

func (m *SessionManager) writeChatSession(instanceID string, req ChatInPayload) error {
	m.mu.RLock()
	csess := m.chatSessions[instanceID]
	m.mu.RUnlock()
	if csess == nil {
		return errors.New("session not found")
	}
	parts := make([]echocli.ContentPart, 0, len(req.ContentParts))
	for _, p := range req.ContentParts {
		cp := echocli.ContentPart{Type: p.Type, Text: p.Text}
		if p.Source != nil {
			cp.Source = &echocli.ImageSource{
				Type:      p.Source.Type,
				MediaType: p.Source.MediaType,
				Data:      p.Source.Data,
			}
		}
		parts = append(parts, cp)
	}
	return csess.Write(echocli.Message{
		MessageID:    req.MessageID,
		Content:      req.Content,
		ContentParts: parts,
	})
}

func (m *SessionManager) sendError(sessionID, instanceID, message string) {
	payload, _ := json.Marshal(map[string]string{
		"message": message,
	})
	msg := NewEnvelope("error", m.cfg.ServerID, sessionID)
	msg.InstanceID = instanceID
	msg.Data = payload
	_ = m.send(msg)
}
