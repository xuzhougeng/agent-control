package core

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

type AgentSender interface {
	Send(msg Envelope) error
}

type Config struct {
	RingBufferBytes   int
	OfflineAfter      time.Duration
	HeartbeatMS       int
	AuditPath         string
	RateLimitPerMin   int
	RateWindow        time.Duration
	DefaultGraceMS    int
	DefaultKillMS     int
	ApprovalBroadcast string // "all" or "attached"
}

type Subscriber struct {
	ID              string
	Actor           string
	Send            chan Envelope
	AttachedSession string
}

type SessionHub struct {
	ring        *RingBuffer
	subscribers map[*Subscriber]struct{}
}

func newSessionHub(ringBytes int) *SessionHub {
	return &SessionHub{
		ring:        NewRingBuffer(ringBytes),
		subscribers: make(map[*Subscriber]struct{}),
	}
}

type ControlPlane struct {
	mu sync.RWMutex

	cfg Config

	servers       map[string]*Server
	sessions      map[string]*Session
	sessionEvents map[string][]SessionEvent
	sessionHubs   map[string]*SessionHub
	agentConns    map[string]AgentSender
	subscribers   map[*Subscriber]struct{}

	detector *PromptDetector
	audit    *AuditLogger
	limiter  *RateLimiter
}

func NewControlPlane(cfg Config) (*ControlPlane, error) {
	if cfg.RingBufferBytes <= 0 {
		cfg.RingBufferBytes = 128 * 1024
	}
	if cfg.OfflineAfter <= 0 {
		cfg.OfflineAfter = 20 * time.Second
	}
	if cfg.HeartbeatMS <= 0 {
		cfg.HeartbeatMS = 5000
	}
	if cfg.DefaultGraceMS <= 0 {
		cfg.DefaultGraceMS = 4000
	}
	if cfg.DefaultKillMS <= 0 {
		cfg.DefaultKillMS = 8000
	}
	if cfg.RateWindow <= 0 {
		cfg.RateWindow = time.Minute
	}
	if cfg.ApprovalBroadcast == "" {
		cfg.ApprovalBroadcast = "all"
	}

	audit, err := NewAuditLogger(cfg.AuditPath)
	if err != nil {
		return nil, err
	}
	cp := &ControlPlane{
		cfg:           cfg,
		servers:       make(map[string]*Server),
		sessions:      make(map[string]*Session),
		sessionEvents: make(map[string][]SessionEvent),
		sessionHubs:   make(map[string]*SessionHub),
		agentConns:    make(map[string]AgentSender),
		subscribers:   make(map[*Subscriber]struct{}),
		detector:      NewPromptDetector(),
		audit:         audit,
		limiter:       NewRateLimiter(cfg.RateLimitPerMin, cfg.RateWindow),
	}
	return cp, nil
}

func (cp *ControlPlane) Close() error {
	if cp.audit != nil {
		return cp.audit.Close()
	}
	return nil
}

func (cp *ControlPlane) RateAllow(token string) bool {
	return cp.limiter.Allow(token)
}

func (cp *ControlPlane) RegisterOrUpdateServer(reg AgentRegister, conn AgentSender) {
	now := time.Now().UnixMilli()
	cp.mu.Lock()
	defer cp.mu.Unlock()

	cp.servers[reg.ServerID] = &Server{
		ServerID:     reg.ServerID,
		Hostname:     reg.Hostname,
		Tags:         append([]string(nil), reg.Tags...),
		OS:           reg.OS,
		Arch:         reg.Arch,
		AgentVersion: reg.AgentVersion,
		LastSeenMS:   now,
		Status:       ServerOnline,
		AllowRoots:   append([]string(nil), reg.AllowRoots...),
		ClaudePath:   reg.ClaudePath,
	}
	cp.agentConns[reg.ServerID] = conn
	cp.audit.Log(AuditEvent{
		Actor:    "agent:" + reg.ServerID,
		ServerID: reg.ServerID,
		Kind:     "register",
	})
}

func (cp *ControlPlane) TouchServer(serverID string) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	s, ok := cp.servers[serverID]
	if !ok {
		return
	}
	s.LastSeenMS = time.Now().UnixMilli()
	s.Status = ServerOnline
}

func (cp *ControlPlane) RemoveAgentConnection(serverID string) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	delete(cp.agentConns, serverID)
	if s, ok := cp.servers[serverID]; ok {
		s.Status = ServerOffline
	}
	cp.audit.Log(AuditEvent{
		Actor:    "agent:" + serverID,
		ServerID: serverID,
		Kind:     "agent_disconnected",
	})
}

func (cp *ControlPlane) GetServers() []Server {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	now := time.Now()
	items := make([]Server, 0, len(cp.servers))
	for _, s := range cp.servers {
		if now.Sub(time.UnixMilli(s.LastSeenMS)) > cp.cfg.OfflineAfter {
			s.Status = ServerOffline
		}
		items = append(items, *s)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ServerID < items[j].ServerID })
	return items
}

func (cp *ControlPlane) GetSessions(serverID string) []Session {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	items := make([]Session, 0, len(cp.sessions))
	for _, s := range cp.sessions {
		if serverID != "" && s.ServerID != serverID {
			continue
		}
		items = append(items, *s)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAtMS > items[j].CreatedAtMS })
	return items
}

func (cp *ControlPlane) GetSessionEvents(sessionID string) []SessionEvent {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	events := cp.sessionEvents[sessionID]
	out := make([]SessionEvent, len(events))
	copy(out, events)
	return out
}

func (cp *ControlPlane) RegisterSubscriber(sub *Subscriber) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.subscribers[sub] = struct{}{}
}

func (cp *ControlPlane) UnregisterSubscriber(sub *Subscriber) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	delete(cp.subscribers, sub)
	if sub.AttachedSession != "" {
		if hub, ok := cp.sessionHubs[sub.AttachedSession]; ok {
			delete(hub.subscribers, sub)
		}
	}
}

func (cp *ControlPlane) AttachSubscriber(sub *Subscriber, sessionID string) ([]byte, uint64, error) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	sess, ok := cp.sessions[sessionID]
	if !ok {
		return nil, 0, errors.New("session not found")
	}
	if sub.AttachedSession != "" {
		if oldHub, ok := cp.sessionHubs[sub.AttachedSession]; ok {
			delete(oldHub.subscribers, sub)
		}
	}
	hub, ok := cp.sessionHubs[sessionID]
	if !ok {
		hub = newSessionHub(cp.cfg.RingBufferBytes)
		cp.sessionHubs[sessionID] = hub
	}
	hub.subscribers[sub] = struct{}{}
	sub.AttachedSession = sessionID
	return hub.ring.Snapshot(), sess.LatestAgentOutSeq, nil
}

func (cp *ControlPlane) CreateSession(actor string, req StartSessionRequest) (*Session, error) {
	if req.ServerID == "" || req.Cwd == "" {
		return nil, errors.New("server_id and cwd are required")
	}
	cp.mu.Lock()
	server, ok := cp.servers[req.ServerID]
	conn := cp.agentConns[req.ServerID]
	if !ok || conn == nil || server.Status != ServerOnline {
		cp.mu.Unlock()
		return nil, errors.New("server offline")
	}
	sessionID := uuid.NewString()
	envKeys := make([]string, 0, len(req.Env))
	for k := range req.Env {
		envKeys = append(envKeys, k)
	}
	sort.Strings(envKeys)
	sess := &Session{
		SessionID:        sessionID,
		ServerID:         req.ServerID,
		Cwd:              req.Cwd,
		Cmd:              []string{"claude-code"},
		EnvKeys:          envKeys,
		Status:           SessionStarting,
		CreatedBy:        actor,
		CreatedAtMS:      time.Now().UnixMilli(),
		AwaitingApproval: false,
	}
	cp.sessions[sessionID] = sess
	cp.sessionHubs[sessionID] = newSessionHub(cp.cfg.RingBufferBytes)
	cp.mu.Unlock()

	payload := map[string]any{
		"cwd":  req.Cwd,
		"cmd":  []string{"claude-code"},
		"env":  req.Env,
		"cols": req.Cols,
		"rows": req.Rows,
	}
	data, _ := json.Marshal(payload)
	msg := NewEnvelope("start_session", req.ServerID, sessionID)
	msg.Data = data
	if err := conn.Send(msg); err != nil {
		cp.mu.Lock()
		sess.Status = SessionError
		sess.ExitReason = "start_session_send_failed"
		cp.mu.Unlock()
		return nil, err
	}
	cp.audit.Log(AuditEvent{
		Actor:     actor,
		ServerID:  req.ServerID,
		SessionID: sessionID,
		Kind:      "create_session",
		Meta: map[string]any{
			"cwd": req.Cwd,
		},
	})
	return sess, nil
}

func (cp *ControlPlane) StopSession(actor, sessionID string, graceMS, killAfterMS int) error {
	cp.mu.Lock()
	sess, ok := cp.sessions[sessionID]
	if !ok {
		cp.mu.Unlock()
		return errors.New("session not found")
	}
	conn := cp.agentConns[sess.ServerID]
	if conn == nil {
		cp.mu.Unlock()
		return errors.New("server offline")
	}
	sess.Status = SessionStopping
	cp.mu.Unlock()

	if graceMS <= 0 {
		graceMS = cp.cfg.DefaultGraceMS
	}
	if killAfterMS <= 0 {
		killAfterMS = cp.cfg.DefaultKillMS
	}
	payload, _ := json.Marshal(map[string]any{
		"grace_ms":      graceMS,
		"kill_after_ms": killAfterMS,
		"signal":        "SIGTERM",
	})
	msg := NewEnvelope("stop_session", sess.ServerID, sessionID)
	msg.Data = payload
	if err := conn.Send(msg); err != nil {
		return err
	}
	cp.audit.Log(AuditEvent{
		Actor:     actor,
		ServerID:  sess.ServerID,
		SessionID: sessionID,
		Kind:      "stop_session",
		Meta: map[string]any{
			"grace_ms":      graceMS,
			"kill_after_ms": killAfterMS,
		},
	})
	cp.broadcastSessionUpdate(sessionID)
	return nil
}

func (cp *ControlPlane) HandlePTYOut(serverID, sessionID string, seq uint64, dataB64 string) {
	raw, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return
	}
	var becameRunning bool
	cp.mu.Lock()
	sess, ok := cp.sessions[sessionID]
	if !ok {
		cp.mu.Unlock()
		return
	}
	if seq > 0 && seq <= sess.LatestAgentOutSeq {
		cp.mu.Unlock()
		return
	}
	if seq > sess.LatestAgentOutSeq {
		sess.LatestAgentOutSeq = seq
	}
	if sess.Status == SessionStarting {
		sess.Status = SessionRunning
		becameRunning = true
	}
	if hub, ok := cp.sessionHubs[sessionID]; ok {
		hub.ring.Write(raw)
	}
	awaiting := sess.AwaitingApproval
	cp.mu.Unlock()

	out := NewEnvelope("term_out", serverID, sessionID)
	out.Seq = seq
	out.DataB64 = dataB64
	cp.broadcastToAttached(sessionID, out)

	if becameRunning {
		cp.broadcastSessionUpdate(sessionID)
	}
	if awaiting {
		return
	}
	matched, excerpt := cp.detector.Feed(sessionID, raw)
	if !matched {
		return
	}
	cp.createApprovalEvent(sessionID, serverID, excerpt)
}

func (cp *ControlPlane) createApprovalEvent(sessionID, serverID, excerpt string) {
	cp.mu.Lock()
	sess, ok := cp.sessions[sessionID]
	if !ok || sess.AwaitingApproval {
		cp.mu.Unlock()
		return
	}
	eventID := uuid.NewString()
	sess.AwaitingApproval = true
	sess.PendingEventID = eventID
	ev := SessionEvent{
		EventID:    eventID,
		SessionID:  sessionID,
		ServerID:   serverID,
		Kind:       "approval_needed",
		PromptText: excerpt,
		TsMS:       time.Now().UnixMilli(),
	}
	cp.sessionEvents[sessionID] = append(cp.sessionEvents[sessionID], ev)
	cp.mu.Unlock()

	body, _ := json.Marshal(ev)
	msg := NewEnvelope("event", serverID, sessionID)
	msg.Data = body
	if cp.cfg.ApprovalBroadcast == "attached" {
		cp.broadcastToAttached(sessionID, msg)
	} else {
		cp.broadcastToAll(msg)
	}
	cp.broadcastSessionUpdate(sessionID)
	cp.audit.Log(AuditEvent{
		Actor:     "system",
		ServerID:  serverID,
		SessionID: sessionID,
		Kind:      "approval_needed",
		Meta: map[string]any{
			"event_id": eventID,
		},
	})
}

func (cp *ControlPlane) HandlePTYExit(serverID, sessionID string, exit PTYExit) {
	cp.mu.Lock()
	sess, ok := cp.sessions[sessionID]
	if !ok {
		cp.mu.Unlock()
		return
	}
	sess.Status = SessionExited
	sess.ExitCode = exit.ExitCode
	sess.ExitReason = exit.Reason
	sess.AwaitingApproval = false
	sess.PendingEventID = ""
	cp.mu.Unlock()

	cp.detector.Clear(sessionID)
	cp.broadcastSessionUpdate(sessionID)
	cp.audit.Log(AuditEvent{
		Actor:     "agent:" + serverID,
		ServerID:  serverID,
		SessionID: sessionID,
		Kind:      "session_exit",
		Meta: map[string]any{
			"reason":    exit.Reason,
			"signal":    exit.Signal,
			"exit_code": exit.ExitCode,
		},
	})
}

func (cp *ControlPlane) HandleClientTermIn(actor, sessionID, dataB64 string) error {
	cp.mu.RLock()
	sess, ok := cp.sessions[sessionID]
	if !ok {
		cp.mu.RUnlock()
		return errors.New("session not found")
	}
	conn := cp.agentConns[sess.ServerID]
	cp.mu.RUnlock()
	if conn == nil {
		return errors.New("server offline")
	}
	msg := NewEnvelope("pty_in", sess.ServerID, sessionID)
	msg.DataB64 = dataB64
	if err := conn.Send(msg); err != nil {
		return err
	}
	raw, _ := base64.StdEncoding.DecodeString(dataB64)
	sum := sha256.Sum256(raw)
	cp.audit.Log(AuditEvent{
		Actor:     actor,
		ServerID:  sess.ServerID,
		SessionID: sessionID,
		Kind:      "term_in",
		Meta: map[string]any{
			"size": len(raw),
			"sha":  hex.EncodeToString(sum[:]),
		},
	})
	return nil
}

func (cp *ControlPlane) HandleClientAction(actor, sessionID string, req ActionRequest) error {
	switch req.Kind {
	case "approve", "reject":
		cp.mu.Lock()
		sess, ok := cp.sessions[sessionID]
		if !ok {
			cp.mu.Unlock()
			return errors.New("session not found")
		}
		if !sess.AwaitingApproval {
			cp.mu.Unlock()
			return errors.New("no pending approval")
		}
		if req.EventID != "" && req.EventID != sess.PendingEventID {
			cp.mu.Unlock()
			return errors.New("event mismatch")
		}
		eventID := sess.PendingEventID
		sess.AwaitingApproval = false
		sess.PendingEventID = ""
		for i := len(cp.sessionEvents[sessionID]) - 1; i >= 0; i-- {
			if cp.sessionEvents[sessionID][i].EventID == eventID {
				cp.sessionEvents[sessionID][i].Resolved = true
				cp.sessionEvents[sessionID][i].Actor = actor
				break
			}
		}
		cp.mu.Unlock()

		input := "y\n"
		if req.Kind == "reject" {
			input = "n\n"
		}
		if err := cp.HandleClientTermIn(actor, sessionID, base64.StdEncoding.EncodeToString([]byte(input))); err != nil {
			return err
		}
		cp.broadcastSessionUpdate(sessionID)
		cp.audit.Log(AuditEvent{
			Actor:     actor,
			SessionID: sessionID,
			Kind:      "action_" + req.Kind,
			Meta: map[string]any{
				"event_id": req.EventID,
			},
		})
		return nil
	case "stop":
		return cp.StopSession(actor, sessionID, cp.cfg.DefaultGraceMS, cp.cfg.DefaultKillMS)
	default:
		return errors.New("invalid action")
	}
}

func (cp *ControlPlane) HandleClientResize(actor, sessionID string, cols, rows uint16) error {
	cp.mu.RLock()
	sess, ok := cp.sessions[sessionID]
	if !ok {
		cp.mu.RUnlock()
		return errors.New("session not found")
	}
	conn := cp.agentConns[sess.ServerID]
	cp.mu.RUnlock()
	if conn == nil {
		return errors.New("server offline")
	}
	body, _ := json.Marshal(map[string]any{"cols": cols, "rows": rows})
	msg := NewEnvelope("resize", sess.ServerID, sessionID)
	msg.Data = body
	if err := conn.Send(msg); err != nil {
		return err
	}
	cp.audit.Log(AuditEvent{
		Actor:     actor,
		ServerID:  sess.ServerID,
		SessionID: sessionID,
		Kind:      "resize",
		Meta: map[string]any{
			"cols": cols,
			"rows": rows,
		},
	})
	return nil
}

func (cp *ControlPlane) broadcastToAttached(sessionID string, msg Envelope) {
	cp.mu.RLock()
	hub := cp.sessionHubs[sessionID]
	if hub == nil {
		cp.mu.RUnlock()
		return
	}
	subs := make([]*Subscriber, 0, len(hub.subscribers))
	for s := range hub.subscribers {
		subs = append(subs, s)
	}
	cp.mu.RUnlock()
	for _, sub := range subs {
		select {
		case sub.Send <- msg:
		default:
		}
	}
}

func (cp *ControlPlane) broadcastToAll(msg Envelope) {
	cp.mu.RLock()
	subs := make([]*Subscriber, 0, len(cp.subscribers))
	for s := range cp.subscribers {
		subs = append(subs, s)
	}
	cp.mu.RUnlock()
	for _, sub := range subs {
		select {
		case sub.Send <- msg:
		default:
		}
	}
}

func (cp *ControlPlane) broadcastSessionUpdate(sessionID string) {
	cp.mu.RLock()
	sess, ok := cp.sessions[sessionID]
	if !ok {
		cp.mu.RUnlock()
		return
	}
	serverID := sess.ServerID
	body, _ := json.Marshal(map[string]any{
		"session_id":        sess.SessionID,
		"status":            sess.Status,
		"exit_code":         sess.ExitCode,
		"exit_reason":       sess.ExitReason,
		"awaiting_approval": sess.AwaitingApproval,
		"pending_event_id":  sess.PendingEventID,
	})
	cp.mu.RUnlock()

	msg := NewEnvelope("session_update", serverID, sessionID)
	msg.Data = body
	cp.broadcastToAll(msg)
}
