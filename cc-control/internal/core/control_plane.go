package core

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strings"
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
	// EnablePromptDetection enables regex-based prompt detection on PTY output to
	// emit "approval_needed" session events. Disabled by default because it's
	// heuristic and may miss prompts depending on the AI CLI/terminal formatting.
	EnablePromptDetection bool
}

type Subscriber struct {
	ID              string
	Actor           string
	Send            chan Envelope
	AttachedSession string
	TenantID        string
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

const MaxChatHistory = 200
const MaxNotificationHistory = 100
const errPTYUnsupportedOnWindows = "PTY is not supported on Windows yet; use session_type=chat"

type ControlPlane struct {
	mu sync.RWMutex

	cfg Config

	servers          map[string]*Server
	sessions         map[string]*Session
	instances        map[string]*RuntimeInstance
	sessionInstances map[string][]string
	sessionEvents    map[string][]SessionEvent
	tenantNotices    map[string][]NotificationEvent
	sessionHubs      map[string]*SessionHub
	agentConns       map[string]AgentSender
	subscribers      map[*Subscriber]struct{}
	chatHistory      map[string][]ChatMessage

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
	var detector *PromptDetector
	if cfg.EnablePromptDetection {
		detector = NewPromptDetector()
	}
	cp := &ControlPlane{
		cfg:              cfg,
		servers:          make(map[string]*Server),
		sessions:         make(map[string]*Session),
		instances:        make(map[string]*RuntimeInstance),
		sessionInstances: make(map[string][]string),
		sessionEvents:    make(map[string][]SessionEvent),
		tenantNotices:    make(map[string][]NotificationEvent),
		sessionHubs:      make(map[string]*SessionHub),
		agentConns:       make(map[string]AgentSender),
		subscribers:      make(map[*Subscriber]struct{}),
		chatHistory:      make(map[string][]ChatMessage),
		detector:         detector,
		audit:            audit,
		limiter:          NewRateLimiter(cfg.RateLimitPerMin, cfg.RateWindow),
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

func (cp *ControlPlane) RegisterOrUpdateServer(tenantID string, reg AgentRegister, conn AgentSender) error {
	now := time.Now().UnixMilli()
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if existing, ok := cp.agentConns[reg.ServerID]; ok && existing != nil {
		return errors.New("duplicate server_id \"" + reg.ServerID + "\": already connected; rename via -server-id")
	}

	cp.servers[reg.ServerID] = &Server{
		TenantID:     tenantID,
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
	return nil
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

func (cp *ControlPlane) GetServers(tenantID string) []Server {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	now := time.Now()
	items := make([]Server, 0, len(cp.servers))
	for _, s := range cp.servers {
		if tenantID != "" && s.TenantID != tenantID {
			continue
		}
		if now.Sub(time.UnixMilli(s.LastSeenMS)) > cp.cfg.OfflineAfter {
			s.Status = ServerOffline
		}
		items = append(items, *s)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ServerID < items[j].ServerID })
	return items
}

func (cp *ControlPlane) GetSessions(tenantID, serverID string) []Session {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	items := make([]Session, 0, len(cp.sessions))
	for _, s := range cp.sessions {
		if tenantID != "" && s.TenantID != tenantID {
			continue
		}
		if serverID != "" && s.ServerID != serverID {
			continue
		}
		items = append(items, *s)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAtMS > items[j].CreatedAtMS })
	return items
}

func (cp *ControlPlane) GetSessionInstances(tenantID, sessionID string) []RuntimeInstance {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	sess, ok := cp.sessions[sessionID]
	if !ok {
		return nil
	}
	if tenantID != "" && sess.TenantID != tenantID {
		return nil
	}
	instanceIDs := cp.sessionInstances[sessionID]
	items := make([]RuntimeInstance, 0, len(instanceIDs))
	for _, instanceID := range instanceIDs {
		inst := cp.instances[instanceID]
		if inst == nil {
			continue
		}
		items = append(items, *inst)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAtMS > items[j].CreatedAtMS })
	return items
}

func (cp *ControlPlane) GetSessionEvents(tenantID, sessionID string) []SessionEvent {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	if tenantID != "" {
		sess, ok := cp.sessions[sessionID]
		if !ok || sess.TenantID != tenantID {
			return nil
		}
	}
	events := cp.sessionEvents[sessionID]
	out := make([]SessionEvent, len(events))
	copy(out, events)
	return out
}

func (cp *ControlPlane) runtimeInstanceForModeLocked(sessionID string, sessionType SessionType) *RuntimeInstance {
	for _, instanceID := range cp.sessionInstances[sessionID] {
		inst := cp.instances[instanceID]
		if inst == nil {
			continue
		}
		if inst.SessionType == sessionType {
			return inst
		}
	}
	return nil
}

func (cp *ControlPlane) createRuntimeInstanceLocked(sess *Session, sessionType SessionType) *RuntimeInstance {
	inst := cp.runtimeInstanceForModeLocked(sess.SessionID, sessionType)
	if inst == nil {
		inst = &RuntimeInstance{
			InstanceID: uuid.NewString(),
			SessionID:  sess.SessionID,
			TenantID:   sess.TenantID,
			ServerID:   sess.ServerID,
		}
		cp.instances[inst.InstanceID] = inst
		cp.sessionInstances[sess.SessionID] = append(cp.sessionInstances[sess.SessionID], inst.InstanceID)
	}
	inst.SessionType = sessionType
	inst.ServerID = sess.ServerID
	inst.Status = SessionStarting
	inst.CreatedAtMS = time.Now().UnixMilli()
	inst.ExitCode = nil
	inst.ExitReason = ""
	inst.AwaitingApproval = false
	inst.PendingEventID = ""
	inst.LatestAgentOutSeq = 0
	sess.ActiveInstanceID = inst.InstanceID
	sess.SessionType = sessionType
	sess.Status = SessionStarting
	sess.ExitCode = nil
	sess.ExitReason = ""
	sess.AwaitingApproval = false
	sess.PendingEventID = ""
	sess.LatestAgentOutSeq = 0
	return inst
}

func (cp *ControlPlane) activeInstanceBySessionLocked(sessionID string) *RuntimeInstance {
	sess := cp.sessions[sessionID]
	if sess == nil || sess.ActiveInstanceID == "" {
		return nil
	}
	return cp.instances[sess.ActiveInstanceID]
}

func (cp *ControlPlane) shouldResumePTYLocked(sessionID string) bool {
	return len(cp.chatHistory[sessionID]) > 0
}

// GetPendingApprovalEvents returns unresolved approval events across all sessions.
func (cp *ControlPlane) GetPendingApprovalEvents(tenantID string) []SessionEvent {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	out := make([]SessionEvent, 0)
	for _, events := range cp.sessionEvents {
		for _, ev := range events {
			if ev.Kind != "approval_needed" || ev.Resolved {
				continue
			}
			if tenantID != "" && ev.TenantID != tenantID {
				continue
			}
			out = append(out, ev)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TsMS == out[j].TsMS {
			return out[i].EventID > out[j].EventID
		}
		return out[i].TsMS > out[j].TsMS
	})
	return out
}

func (cp *ControlPlane) GetRecentNotificationEvents(tenantID string, limit int) []NotificationEvent {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	if limit <= 0 || limit > MaxNotificationHistory {
		limit = MaxNotificationHistory
	}
	events := cp.tenantNotices[tenantID]
	if len(events) == 0 {
		return nil
	}
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	out := make([]NotificationEvent, len(events))
	copy(out, events)
	return out
}

func (cp *ControlPlane) PublishNotification(actor, tenantID string, req PublishNotificationRequest) (NotificationEvent, error) {
	if tenantID == "" {
		return NotificationEvent{}, errors.New("tenant id required")
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		return NotificationEvent{}, errors.New("message is required")
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Source = strings.TrimSpace(req.Source)
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.ServerID = strings.TrimSpace(req.ServerID)
	if req.Source == "" {
		req.Source = "external"
	}

	level := req.Level
	switch level {
	case NotificationLevelInfo, NotificationLevelSuccess, NotificationLevelWarning, NotificationLevelError:
	default:
		level = NotificationLevelInfo
	}

	ev := NotificationEvent{
		NotificationID: uuid.NewString(),
		Kind:           "notification",
		TenantID:       tenantID,
		SessionID:      req.SessionID,
		ServerID:       req.ServerID,
		Level:          level,
		Title:          req.Title,
		Message:        req.Message,
		Source:         req.Source,
		Actor:          actor,
		TsMS:           time.Now().UnixMilli(),
	}

	cp.mu.Lock()
	if ev.SessionID != "" {
		sess, ok := cp.sessions[ev.SessionID]
		if !ok || sess.TenantID != tenantID {
			cp.mu.Unlock()
			return NotificationEvent{}, errors.New("session not found")
		}
		if ev.ServerID == "" {
			ev.ServerID = sess.ServerID
		}
	}
	if ev.ServerID != "" {
		srv, ok := cp.servers[ev.ServerID]
		if !ok || srv.TenantID != tenantID {
			cp.mu.Unlock()
			return NotificationEvent{}, errors.New("server not found")
		}
	}
	events := append(cp.tenantNotices[tenantID], ev)
	if len(events) > MaxNotificationHistory {
		events = events[len(events)-MaxNotificationHistory:]
	}
	cp.tenantNotices[tenantID] = events
	cp.mu.Unlock()

	body, _ := json.Marshal(ev)
	msg := NewEnvelope("event", ev.ServerID, ev.SessionID)
	msg.Data = body
	cp.broadcastToTenant(tenantID, msg)

	cp.audit.Log(AuditEvent{
		Actor:     actor,
		ServerID:  ev.ServerID,
		SessionID: ev.SessionID,
		Kind:      "notification_publish",
		Meta: map[string]any{
			"notification_id": ev.NotificationID,
			"level":           ev.Level,
			"source":          ev.Source,
		},
	})
	return ev, nil
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
	if sub.TenantID != "" && sess.TenantID != sub.TenantID {
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

func (cp *ControlPlane) CreateSession(actor string, tenantID string, req StartSessionRequest) (*Session, error) {
	if req.ServerID == "" || req.Cwd == "" {
		return nil, errors.New("server_id and cwd are required")
	}
	sessType := req.SessionType
	if sessType != "" && sessType != SessionTypePTY && sessType != SessionTypeChat {
		return nil, errors.New("invalid session_type")
	}
	cp.mu.Lock()
	server, ok := cp.servers[req.ServerID]
	conn := cp.agentConns[req.ServerID]
	if !ok || conn == nil || server.Status != ServerOnline {
		cp.mu.Unlock()
		return nil, errors.New("server offline")
	}
	if tenantID != "" && server.TenantID != tenantID {
		cp.mu.Unlock()
		return nil, errors.New("server not in tenant")
	}
	if sessType == "" {
		if strings.EqualFold(server.OS, "windows") {
			sessType = SessionTypeChat
		} else {
			sessType = SessionTypePTY
		}
	}
	if sessType != SessionTypePTY && sessType != SessionTypeChat {
		cp.mu.Unlock()
		return nil, errors.New("invalid session_type")
	}
	if sessType == SessionTypePTY && strings.EqualFold(server.OS, "windows") {
		cp.mu.Unlock()
		return nil, errors.New(errPTYUnsupportedOnWindows)
	}
	sessionID := strings.ToLower(strings.TrimSpace(req.SessionID))
	if sessionID == "" {
		sessionID = uuid.NewString()
	} else {
		if _, err := uuid.Parse(sessionID); err != nil {
			cp.mu.Unlock()
			return nil, errors.New("invalid session_id")
		}
	}
	if _, exists := cp.sessions[sessionID]; exists {
		cp.mu.Unlock()
		return nil, errors.New("session_id already exists")
	}

	envKeys := make([]string, 0, len(req.Env))
	for k := range req.Env {
		envKeys = append(envKeys, k)
	}
	sort.Strings(envKeys)
	sess := &Session{
		TenantID:         tenantID,
		SessionID:        sessionID,
		ServerID:         req.ServerID,
		SessionType:      sessType,
		Cwd:              req.Cwd,
		Cmd:              nil,
		EnvKeys:          envKeys,
		Status:           SessionStarting,
		CreatedBy:        actor,
		CreatedAtMS:      time.Now().UnixMilli(),
		AwaitingApproval: false,
	}
	cp.sessions[sessionID] = sess
	cp.sessionHubs[sessionID] = newSessionHub(cp.cfg.RingBufferBytes)
	inst := cp.createRuntimeInstanceLocked(sess, sessType)
	resumePTY := sessType == SessionTypePTY && cp.shouldResumePTYLocked(sessionID) && len(cp.sessionInstances[sessionID]) > 1
	cp.mu.Unlock()

	if err := cp.sendStartInstance(conn, server, sess, inst, req.Env, req.Cols, req.Rows, resumePTY); err != nil {
		cp.mu.Lock()
		sess.Status = SessionError
		inst.Status = SessionError
		reason := string(sess.SessionType) + "_send_failed"
		sess.ExitReason = reason
		inst.ExitReason = reason
		cp.mu.Unlock()
		return nil, err
	}
	cp.audit.Log(AuditEvent{
		Actor:     actor,
		ServerID:  req.ServerID,
		SessionID: sessionID,
		Kind:      "create_session",
		Meta: map[string]any{
			"cwd":          req.Cwd,
			"session_type": string(sessType),
		},
	})
	return sess, nil
}

func (cp *ControlPlane) sendStartInstance(conn AgentSender, server *Server, sess *Session, inst *RuntimeInstance, env map[string]string, cols, rows uint16, resumePTY bool) error {
	var msgType string
	var data []byte
	if inst.SessionType == SessionTypeChat {
		sess.Cmd = nil
		msgType = "start_chat"
		payload := map[string]any{
			"cwd": sess.Cwd,
			"env": env,
		}
		data, _ = json.Marshal(payload)
	} else {
		cmdPath := strings.TrimSpace(server.ClaudePath)
		if cmdPath == "" {
			cmdPath = "claude-code"
		}
		cmd := []string{cmdPath}
		if supportsClaudeSessionFlags(cmdPath) {
			if resumePTY {
				cmd = append(cmd, "--resume", sess.SessionID)
			} else {
				cmd = append(cmd, "--session-id", sess.SessionID)
			}
		}
		sess.Cmd = append([]string(nil), cmd...)
		payload := map[string]any{
			"cwd":  sess.Cwd,
			"cmd":  cmd,
			"env":  env,
			"cols": cols,
			"rows": rows,
		}
		msgType = "start_session"
		data, _ = json.Marshal(payload)
	}
	msg := NewEnvelope(msgType, sess.ServerID, sess.SessionID)
	msg.InstanceID = inst.InstanceID
	msg.Data = data
	return conn.Send(msg)
}

func supportsClaudeSessionFlags(cmdPath string) bool {
	base := normalizedCommandBase(cmdPath)
	if base == "" {
		return false
	}
	return base == "claude" || base == "claude-code"
}

func normalizedCommandBase(cmdPath string) string {
	cmdPath = strings.TrimSpace(cmdPath)
	if cmdPath == "" {
		return ""
	}
	if idx := strings.LastIndexAny(cmdPath, `/\`); idx >= 0 {
		cmdPath = cmdPath[idx+1:]
	}
	cmdPath = strings.ToLower(strings.TrimSpace(cmdPath))
	for _, ext := range []string{".exe", ".cmd", ".bat", ".ps1", ".sh"} {
		cmdPath = strings.TrimSuffix(cmdPath, ext)
	}
	return cmdPath
}

func (cp *ControlPlane) SwitchSessionMode(actor, tenantID, sessionID string, req SwitchSessionRequest) (*Session, error) {
	targetType := req.SessionType
	if targetType != SessionTypePTY && targetType != SessionTypeChat {
		return nil, errors.New("invalid session_type")
	}

	cp.mu.RLock()
	sess, ok := cp.sessions[sessionID]
	if !ok {
		cp.mu.RUnlock()
		return nil, errors.New("session not found")
	}
	if tenantID != "" && sess.TenantID != tenantID {
		cp.mu.RUnlock()
		return nil, errors.New("session not found")
	}
	server, ok := cp.servers[sess.ServerID]
	conn := cp.agentConns[sess.ServerID]
	if !ok || conn == nil || server.Status != ServerOnline {
		cp.mu.RUnlock()
		return nil, errors.New("server offline")
	}
	if targetType == SessionTypePTY && strings.EqualFold(server.OS, "windows") {
		cp.mu.RUnlock()
		return nil, errors.New(errPTYUnsupportedOnWindows)
	}
	currentType := sess.SessionType
	currentStatus := sess.Status
	cp.mu.RUnlock()

	if currentType == targetType && (currentStatus == SessionStarting || currentStatus == SessionRunning) && len(req.Env) == 0 {
		cp.mu.RLock()
		out := *cp.sessions[sessionID]
		cp.mu.RUnlock()
		return &out, nil
	}

	if currentStatus == SessionStarting || currentStatus == SessionRunning || currentStatus == SessionStopping {
		if err := cp.StopSession(actor, tenantID, sessionID, 0, 0); err != nil {
			return nil, err
		}
		if err := cp.waitForSessionStop(tenantID, sessionID, cp.stopWaitTimeout(0, 0)); err != nil {
			return nil, err
		}
	}

	cp.mu.Lock()
	sess, ok = cp.sessions[sessionID]
	if !ok {
		cp.mu.Unlock()
		return nil, errors.New("session not found")
	}
	server = cp.servers[sess.ServerID]
	conn = cp.agentConns[sess.ServerID]
	if server == nil || conn == nil || server.Status != ServerOnline {
		cp.mu.Unlock()
		return nil, errors.New("server offline")
	}
	envKeys := make([]string, 0, len(req.Env))
	for k := range req.Env {
		envKeys = append(envKeys, k)
	}
	sort.Strings(envKeys)
	sess.EnvKeys = envKeys
	resumePTY := targetType == SessionTypePTY && cp.shouldResumePTYLocked(sessionID)
	inst := cp.createRuntimeInstanceLocked(sess, targetType)
	cp.mu.Unlock()

	if err := cp.sendStartInstance(conn, server, sess, inst, req.Env, req.Cols, req.Rows, resumePTY); err != nil {
		cp.mu.Lock()
		sess.Status = SessionError
		inst.Status = SessionError
		reason := string(targetType) + "_send_failed"
		sess.ExitReason = reason
		inst.ExitReason = reason
		cp.mu.Unlock()
		return nil, err
	}
	cp.broadcastSessionUpdate(sessionID)
	cp.audit.Log(AuditEvent{
		Actor:     actor,
		ServerID:  sess.ServerID,
		SessionID: sessionID,
		Kind:      "switch_session",
		Meta: map[string]any{
			"session_type": targetType,
			"instance_id":  inst.InstanceID,
		},
	})

	cp.mu.RLock()
	out := *cp.sessions[sessionID]
	cp.mu.RUnlock()
	return &out, nil
}

func (cp *ControlPlane) StopSession(actor, tenantID, sessionID string, graceMS, killAfterMS int) error {
	cp.mu.Lock()
	sess, ok := cp.sessions[sessionID]
	if !ok {
		cp.mu.Unlock()
		return errors.New("session not found")
	}
	if tenantID != "" && sess.TenantID != tenantID {
		cp.mu.Unlock()
		return errors.New("session not found")
	}
	inst := cp.activeInstanceBySessionLocked(sessionID)
	if inst == nil {
		cp.mu.Unlock()
		return errors.New("active instance not found")
	}
	conn := cp.agentConns[sess.ServerID]
	if conn == nil {
		cp.mu.Unlock()
		return errors.New("server offline")
	}
	sess.Status = SessionStopping
	inst.Status = SessionStopping
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
	msg.InstanceID = inst.InstanceID
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

func (cp *ControlPlane) DeleteSession(actor, tenantID, sessionID string) error {
	cp.mu.Lock()
	sess, ok := cp.sessions[sessionID]
	if !ok {
		cp.mu.Unlock()
		return errors.New("session not found")
	}
	if tenantID != "" && sess.TenantID != tenantID {
		cp.mu.Unlock()
		return errors.New("session not found")
	}
	if sess.Status == SessionStarting || sess.Status == SessionRunning || sess.Status == SessionStopping {
		cp.mu.Unlock()
		return errors.New("session still active; stop first")
	}
	delete(cp.sessions, sessionID)
	for _, instanceID := range cp.sessionInstances[sessionID] {
		delete(cp.instances, instanceID)
	}
	delete(cp.sessionInstances, sessionID)
	delete(cp.sessionEvents, sessionID)
	delete(cp.sessionHubs, sessionID)
	delete(cp.chatHistory, sessionID)
	for sub := range cp.subscribers {
		if sub.AttachedSession == sessionID {
			sub.AttachedSession = ""
		}
	}
	cp.mu.Unlock()

	if cp.detector != nil {
		cp.detector.Clear(sessionID)
	}
	cp.audit.Log(AuditEvent{
		Actor:     actor,
		ServerID:  sess.ServerID,
		SessionID: sessionID,
		Kind:      "delete_session",
	})
	return nil
}

func (cp *ControlPlane) StopAndDeleteSession(actor, tenantID, sessionID string, graceMS, killAfterMS int) error {
	cp.mu.RLock()
	sess, ok := cp.sessions[sessionID]
	if !ok {
		cp.mu.RUnlock()
		return errors.New("session not found")
	}
	if tenantID != "" && sess.TenantID != tenantID {
		cp.mu.RUnlock()
		return errors.New("session not found")
	}
	status := sess.Status
	cp.mu.RUnlock()

	if status == SessionStarting || status == SessionRunning || status == SessionStopping {
		if err := cp.StopSession(actor, tenantID, sessionID, graceMS, killAfterMS); err != nil {
			return err
		}
		if err := cp.waitForSessionStop(tenantID, sessionID, cp.stopWaitTimeout(graceMS, killAfterMS)); err != nil {
			return err
		}
	}

	return cp.DeleteSession(actor, tenantID, sessionID)
}

func (cp *ControlPlane) stopWaitTimeout(graceMS, killAfterMS int) time.Duration {
	if graceMS <= 0 {
		graceMS = cp.cfg.DefaultGraceMS
	}
	if killAfterMS <= 0 {
		killAfterMS = cp.cfg.DefaultKillMS
	}
	waitMS := killAfterMS
	if graceMS > waitMS {
		waitMS = graceMS
	}
	if waitMS < 1000 {
		waitMS = 1000
	}
	return time.Duration(waitMS+1500) * time.Millisecond
}

func (cp *ControlPlane) waitForSessionStop(tenantID, sessionID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		cp.mu.RLock()
		sess, ok := cp.sessions[sessionID]
		if !ok {
			cp.mu.RUnlock()
			return nil
		}
		if tenantID != "" && sess.TenantID != tenantID {
			cp.mu.RUnlock()
			return errors.New("session not found")
		}
		status := sess.Status
		reason := strings.TrimSpace(sess.ExitReason)
		cp.mu.RUnlock()

		if status == SessionExited {
			return nil
		}
		if status == SessionError {
			if reason == "" {
				return errors.New("session stop failed")
			}
			return errors.New(reason)
		}
		if time.Now().After(deadline) {
			return errors.New("timed out waiting for session to stop")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (cp *ControlPlane) HandlePTYOut(serverID, sessionID, instanceID string, seq uint64, dataB64 string) {
	raw, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return
	}
	var becameRunning bool
	var awaiting bool
	var active bool
	cp.mu.Lock()
	sess, ok := cp.sessions[sessionID]
	if !ok {
		cp.mu.Unlock()
		return
	}
	inst := cp.instances[instanceID]
	if inst == nil || inst.SessionID != sessionID {
		cp.mu.Unlock()
		return
	}
	if seq > 0 && seq <= inst.LatestAgentOutSeq {
		cp.mu.Unlock()
		return
	}
	if seq > inst.LatestAgentOutSeq {
		inst.LatestAgentOutSeq = seq
	}
	active = sess.ActiveInstanceID == instanceID
	if active {
		sess.LatestAgentOutSeq = inst.LatestAgentOutSeq
		if sess.Status == SessionStarting {
			sess.Status = SessionRunning
			becameRunning = true
		}
		if hub, ok := cp.sessionHubs[sessionID]; ok {
			hub.ring.Write(raw)
		}
		awaiting = sess.AwaitingApproval
	}
	if inst.Status == SessionStarting {
		inst.Status = SessionRunning
	}
	cp.mu.Unlock()

	if !active {
		return
	}

	out := NewEnvelope("term_out", serverID, sessionID)
	out.InstanceID = instanceID
	out.Seq = seq
	out.DataB64 = dataB64
	cp.broadcastToAttached(sessionID, out)

	if becameRunning {
		cp.broadcastSessionUpdate(sessionID)
	}
	if awaiting || cp.detector == nil {
		return
	}
	matched, excerpt := cp.detector.Feed(sessionID, raw)
	if !matched {
		return
	}
	cp.createApprovalEvent(sessionID, instanceID, serverID, excerpt)
}

func (cp *ControlPlane) createApprovalEvent(sessionID, instanceID, serverID, excerpt string) {
	cp.createApprovalEventInner(sessionID, instanceID, serverID, excerpt, "")
}

// HandleAgentApprovalRequest is the entry point for cc-agent's RemoteApprover.
// It records the agent request id alongside a normal approval_needed
// SessionEvent so the existing UI Pending Approval card surfaces it. The
// user's approve/reject is routed back to the agent via SendApprovalDecision
// instead of `y\n` / `n\n` term_in.
func (cp *ControlPlane) HandleAgentApprovalRequest(serverID, sessionID, instanceID, requestID, command, reason string) {
	if requestID == "" {
		slog.Warn("agent approval request ignored: empty request_id", "server_id", serverID, "session_id", sessionID)
		return
	}
	cp.mu.Lock()
	sess, sessOK := cp.sessions[sessionID]
	var awaiting bool
	var activeIID string
	if sessOK {
		awaiting = sess.AwaitingApproval
		activeIID = sess.ActiveInstanceID
	}
	cp.mu.Unlock()
	slog.Info("agent approval request received",
		"server_id", serverID, "session_id", sessionID, "instance_id", instanceID,
		"request_id", requestID, "session_known", sessOK,
		"awaiting", awaiting, "active_instance_id", activeIID,
		"reason", reason)
	excerpt := command
	if reason != "" {
		excerpt = "[" + reason + "] " + command
	}
	cp.createApprovalEventInner(sessionID, instanceID, serverID, excerpt, requestID)
}

func (cp *ControlPlane) createApprovalEventInner(sessionID, instanceID, serverID, excerpt, agentRequestID string) {
	cp.mu.Lock()
	sess, ok := cp.sessions[sessionID]
	if !ok || sess.AwaitingApproval || sess.ActiveInstanceID != instanceID {
		var awaiting bool
		var activeIID string
		if sess != nil {
			awaiting = sess.AwaitingApproval
			activeIID = sess.ActiveInstanceID
		}
		cp.mu.Unlock()
		slog.Warn("approval event suppressed",
			"session_id", sessionID, "session_found", ok,
			"awaiting", awaiting, "active_instance", activeIID, "got_instance", instanceID,
			"agent_request_id", agentRequestID)
		return
	}
	inst := cp.instances[instanceID]
	if inst == nil {
		cp.mu.Unlock()
		return
	}
	eventID := uuid.NewString()
	sess.AwaitingApproval = true
	sess.PendingEventID = eventID
	inst.AwaitingApproval = true
	inst.PendingEventID = eventID
	ev := SessionEvent{
		EventID:        eventID,
		SessionID:      sessionID,
		InstanceID:     instanceID,
		ServerID:       serverID,
		TenantID:       sess.TenantID,
		Kind:           "approval_needed",
		PromptText:     excerpt,
		TsMS:           time.Now().UnixMilli(),
		AgentRequestID: agentRequestID,
	}
	cp.sessionEvents[sessionID] = append(cp.sessionEvents[sessionID], ev)
	cp.mu.Unlock()

	// Clear the detector buffer so the same prompt text sitting in the ring
	// buffer won't re-trigger a new approval on the next pty_out chunk.
	if cp.detector != nil {
		cp.detector.Clear(sessionID)
	}

	body, _ := json.Marshal(ev)
	msg := NewEnvelope("event", serverID, sessionID)
	msg.InstanceID = instanceID
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

func (cp *ControlPlane) HandlePTYExit(serverID, sessionID, instanceID string, exit PTYExit) {
	var active bool
	cp.mu.Lock()
	sess, ok := cp.sessions[sessionID]
	if !ok {
		cp.mu.Unlock()
		return
	}
	inst := cp.instances[instanceID]
	if inst == nil || inst.SessionID != sessionID {
		cp.mu.Unlock()
		return
	}
	inst.Status = SessionExited
	inst.ExitCode = exit.ExitCode
	inst.ExitReason = exit.Reason
	inst.AwaitingApproval = false
	inst.PendingEventID = ""
	active = sess.ActiveInstanceID == instanceID
	if active {
		sess.Status = SessionExited
		sess.ExitCode = exit.ExitCode
		sess.ExitReason = exit.Reason
		sess.AwaitingApproval = false
		sess.PendingEventID = ""
	}
	cp.mu.Unlock()

	if !active {
		return
	}
	if cp.detector != nil {
		cp.detector.Clear(sessionID)
	}
	cp.broadcastSessionUpdate(sessionID)
	cp.audit.Log(AuditEvent{
		Actor:     "agent:" + serverID,
		ServerID:  serverID,
		SessionID: sessionID,
		Kind:      "session_exit",
		Meta: map[string]any{
			"instance_id": instanceID,
			"reason":      exit.Reason,
			"signal":      exit.Signal,
			"exit_code":   exit.ExitCode,
		},
	})
}

// SendApprovalDecision relays the operator's approve/reject choice back to a
// cc-agent that opened a destructive-command approval. Routed by request_id;
// the agent's RemoteApprover unblocks the matching tool call.
func (cp *ControlPlane) SendApprovalDecision(serverID, sessionID, instanceID, requestID string, approved bool, actor string) error {
	if requestID == "" {
		return errors.New("send approval decision: empty request id")
	}
	cp.mu.Lock()
	conn := cp.agentConns[serverID]
	cp.mu.Unlock()
	if conn == nil {
		return errors.New("agent offline")
	}
	payload, _ := json.Marshal(map[string]any{
		"request_id": requestID,
		"approved":   approved,
		"actor":      actor,
	})
	env := NewEnvelope("approval_decision", serverID, sessionID)
	env.InstanceID = instanceID
	env.Data = payload
	return conn.Send(env)
}

func (cp *ControlPlane) HandleAgentError(serverID, sessionID, instanceID, message string) {
	if sessionID == "" {
		return
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "agent error"
	}

	// Avoid marking a session as failed due to transient early messages (e.g. resize before start_session completes).
	// For sessions still "starting", only treat explicit start failures as fatal.
	note := "\r\n[agent error] " + message + "\r\n"

	var (
		hub    *SessionHub
		latest uint64
		status SessionStatus
		active bool
	)

	cp.mu.Lock()
	sess, ok := cp.sessions[sessionID]
	if !ok {
		cp.mu.Unlock()
		return
	}
	inst := cp.instances[instanceID]
	if inst == nil || inst.SessionID != sessionID {
		cp.mu.Unlock()
		return
	}
	active = sess.ActiveInstanceID == instanceID
	status = inst.Status
	if !active && status != SessionStarting {
		cp.mu.Unlock()
		return
	}
	if active {
		status = sess.Status
	}
	if active && (status == SessionError || status == SessionExited) {
		cp.mu.Unlock()
		return
	}
	if status == SessionStarting {
		// During startup, ignore transient errors caused by early messages (e.g. resize/term_in)
		// arriving before the agent finishes creating the PTY session.
		// Everything else should fail the session fast to avoid being stuck in "starting" forever.
		if message == "session not found" {
			cp.mu.Unlock()
			return
		}
	}

	inst.Status = SessionError
	if inst.ExitReason == "" {
		inst.ExitReason = message
	}
	inst.AwaitingApproval = false
	inst.PendingEventID = ""
	latest = inst.LatestAgentOutSeq
	if active {
		sess.Status = SessionError
		if sess.ExitReason == "" {
			sess.ExitReason = message
		}
		sess.AwaitingApproval = false
		sess.PendingEventID = ""
		sess.LatestAgentOutSeq = latest
		hub = cp.sessionHubs[sessionID]
	}
	cp.mu.Unlock()

	if !active {
		return
	}
	if hub != nil {
		hub.ring.Write([]byte(note))
	}
	out := NewEnvelope("term_out", serverID, sessionID)
	out.InstanceID = instanceID
	out.Seq = latest
	out.DataB64 = base64.StdEncoding.EncodeToString([]byte(note))
	cp.broadcastToAttached(sessionID, out)

	if cp.detector != nil {
		cp.detector.Clear(sessionID)
	}
	cp.broadcastSessionUpdate(sessionID)
	cp.audit.Log(AuditEvent{
		Actor:     "agent:" + serverID,
		ServerID:  serverID,
		SessionID: sessionID,
		Kind:      "agent_error",
		Meta: map[string]any{
			"instance_id": instanceID,
			"message":     message,
		},
	})
}

func (cp *ControlPlane) HandleClientTermIn(actor, tenantID, sessionID, dataB64 string) error {
	cp.mu.RLock()
	sess, ok := cp.sessions[sessionID]
	if !ok {
		cp.mu.RUnlock()
		return errors.New("session not found")
	}
	if tenantID != "" && sess.TenantID != tenantID {
		cp.mu.RUnlock()
		return errors.New("session not found")
	}
	if sess.SessionType != SessionTypePTY {
		cp.mu.RUnlock()
		return errors.New("session is not a pty session")
	}
	inst := cp.activeInstanceBySessionLocked(sessionID)
	if inst == nil {
		cp.mu.RUnlock()
		return errors.New("active instance not found")
	}
	conn := cp.agentConns[sess.ServerID]
	cp.mu.RUnlock()
	if conn == nil {
		return errors.New("server offline")
	}
	msg := NewEnvelope("pty_in", sess.ServerID, sessionID)
	msg.InstanceID = inst.InstanceID
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

func (cp *ControlPlane) HandleClientAction(actor, tenantID, sessionID string, req ActionRequest) error {
	switch req.Kind {
	case "approve", "reject":
		cp.mu.Lock()
		sess, ok := cp.sessions[sessionID]
		if !ok {
			cp.mu.Unlock()
			return errors.New("session not found")
		}
		if tenantID != "" && sess.TenantID != tenantID {
			cp.mu.Unlock()
			return errors.New("session not found")
		}
		if !sess.AwaitingApproval {
			cp.mu.Unlock()
			return errors.New("no pending approval")
		}
		// Clients may submit a stale event_id after reconnect; always execute
		// against the session's current pending event for robustness.
		requestedEventID := req.EventID
		eventID := sess.PendingEventID
		var promptExcerpt string
		var agentRequestID string
		sess.AwaitingApproval = false
		sess.PendingEventID = ""
		if inst := cp.activeInstanceBySessionLocked(sessionID); inst != nil {
			inst.AwaitingApproval = false
			inst.PendingEventID = ""
		}
		for i := len(cp.sessionEvents[sessionID]) - 1; i >= 0; i-- {
			if cp.sessionEvents[sessionID][i].EventID == eventID {
				promptExcerpt = cp.sessionEvents[sessionID][i].PromptText
				agentRequestID = cp.sessionEvents[sessionID][i].AgentRequestID
				cp.sessionEvents[sessionID][i].Resolved = true
				cp.sessionEvents[sessionID][i].Actor = actor
				break
			}
		}
		serverIDForAction := sess.ServerID
		instanceForAction := sess.ActiveInstanceID
		cp.mu.Unlock()

		// cc-agent flow: send a structured decision back to the agent.
		if agentRequestID != "" {
			if err := cp.SendApprovalDecision(serverIDForAction, sessionID, instanceForAction, agentRequestID, req.Kind == "approve", actor); err != nil {
				return err
			}
			cp.broadcastSessionUpdate(sessionID)
			cp.audit.Log(AuditEvent{
				Actor:     actor,
				ServerID:  serverIDForAction,
				SessionID: sessionID,
				Kind:      "action_" + req.Kind,
				Meta: map[string]any{
					"event_id":           eventID,
					"requested_event_id": requestedEventID,
					"agent_request_id":   agentRequestID,
				},
			})
			return nil
		}

		input := "y\n"
		if req.Kind == "reject" {
			input = "n\n"
		}
		// Claude Code / Cursor-style approval menus are not y/n; approving is Enter (default "Yes"),
		// rejecting is Esc (cancel).
		if looksLikeApprovalMenuPrompt(promptExcerpt) {
			if req.Kind == "approve" {
				input = "\r"
			} else {
				input = "\u001b"
			}
		}
		if err := cp.HandleClientTermIn(actor, tenantID, sessionID, base64.StdEncoding.EncodeToString([]byte(input))); err != nil {
			return err
		}
		cp.broadcastSessionUpdate(sessionID)
		cp.audit.Log(AuditEvent{
			Actor:     actor,
			SessionID: sessionID,
			Kind:      "action_" + req.Kind,
			Meta: map[string]any{
				"event_id":           eventID,
				"requested_event_id": requestedEventID,
			},
		})
		return nil
	case "stop":
		return cp.StopSession(actor, tenantID, sessionID, cp.cfg.DefaultGraceMS, cp.cfg.DefaultKillMS)
	default:
		return errors.New("invalid action")
	}
}

func looksLikeApprovalMenuPrompt(prompt string) bool {
	p := normalizePromptForMenuMatch(prompt)
	if p == "" {
		return false
	}
	// Most reliable menu marker in Claude Code style prompts.
	if strings.Contains(p, "esc to cancel") && strings.Contains(p, "tab to amend") {
		return true
	}
	// Generic numbered menu patterns:
	// 1. Yes / 1) Yes
	// 2. ... and/or 3. No
	hasFirstYes := strings.Contains(p, "1. yes") || strings.Contains(p, "1) yes")
	hasSecond := strings.Contains(p, "2. ") || strings.Contains(p, "2) ")
	hasThirdNo := strings.Contains(p, "3. no") || strings.Contains(p, "3) no")
	if strings.Contains(p, "do you want to") && hasFirstYes && (hasSecond || hasThirdNo) {
		return true
	}
	if strings.Contains(p, "always allow access") && strings.Contains(p, "from this project") {
		return true
	}
	return false
}

func normalizePromptForMenuMatch(prompt string) string {
	if strings.TrimSpace(prompt) == "" {
		return ""
	}
	// Normalize whitespace and case so matching is stable against terminal formatting.
	return strings.Join(strings.Fields(strings.ToLower(prompt)), " ")
}

func (cp *ControlPlane) HandleClientResize(actor, tenantID, sessionID string, cols, rows uint16) error {
	cp.mu.RLock()
	sess, ok := cp.sessions[sessionID]
	if !ok {
		cp.mu.RUnlock()
		return errors.New("session not found")
	}
	if tenantID != "" && sess.TenantID != tenantID {
		cp.mu.RUnlock()
		return errors.New("session not found")
	}
	if sess.SessionType != SessionTypePTY {
		cp.mu.RUnlock()
		return errors.New("session is not a pty session")
	}
	inst := cp.activeInstanceBySessionLocked(sessionID)
	if inst == nil {
		cp.mu.RUnlock()
		return errors.New("active instance not found")
	}
	conn := cp.agentConns[sess.ServerID]
	cp.mu.RUnlock()
	if conn == nil {
		return errors.New("server offline")
	}
	body, _ := json.Marshal(map[string]any{"cols": cols, "rows": rows})
	msg := NewEnvelope("resize", sess.ServerID, sessionID)
	msg.InstanceID = inst.InstanceID
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

func sanitizeChatContentParts(content string, parts []ChatContentPart) ([]ChatContentPart, error) {
	if len(parts) == 0 {
		content = strings.TrimSpace(content)
		if content == "" {
			return nil, errors.New("empty chat message")
		}
		return []ChatContentPart{{Type: "text", Text: content}}, nil
	}

	out := make([]ChatContentPart, 0, len(parts)+1)
	for _, part := range parts {
		switch strings.ToLower(strings.TrimSpace(part.Type)) {
		case "text":
			text := strings.TrimSpace(part.Text)
			if text == "" {
				continue
			}
			out = append(out, ChatContentPart{Type: "text", Text: text})
		case "image":
			if part.Source == nil {
				continue
			}
			sourceType := strings.ToLower(strings.TrimSpace(part.Source.Type))
			mediaType := strings.ToLower(strings.TrimSpace(part.Source.MediaType))
			data := strings.TrimSpace(part.Source.Data)
			if sourceType != "base64" || mediaType == "" || data == "" {
				continue
			}
			if !strings.HasPrefix(mediaType, "image/") {
				continue
			}
			if _, err := base64.StdEncoding.DecodeString(data); err != nil {
				continue
			}
			out = append(out, ChatContentPart{
				Type: "image",
				Source: &ChatImageSource{
					Type:      "base64",
					MediaType: mediaType,
					Data:      data,
				},
			})
		}
	}
	if len(out) == 0 {
		return nil, errors.New("empty chat message")
	}
	return out, nil
}

func chatPartsPlainText(parts []ChatContentPart) string {
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Type == "text" {
			text := strings.TrimSpace(part.Text)
			if text != "" {
				lines = append(lines, text)
			}
			continue
		}
		if part.Type == "image" && part.Source != nil {
			lines = append(lines, "[image:"+part.Source.MediaType+"]")
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n\n"))
}

func (cp *ControlPlane) HandleClientChatIn(actor, tenantID, sessionID, content string, contentParts []ChatContentPart) error {
	parts, err := sanitizeChatContentParts(content, contentParts)
	if err != nil {
		return err
	}
	plainContent := chatPartsPlainText(parts)
	meta, _ := json.Marshal(map[string]any{
		"content_parts": parts,
	})

	cp.mu.Lock()
	sess, ok := cp.sessions[sessionID]
	if !ok {
		cp.mu.Unlock()
		return errors.New("session not found")
	}
	if tenantID != "" && sess.TenantID != tenantID {
		cp.mu.Unlock()
		return errors.New("session not found")
	}
	inst := cp.activeInstanceBySessionLocked(sessionID)
	if inst == nil {
		cp.mu.Unlock()
		return errors.New("active instance not found")
	}
	if sess.SessionType != SessionTypeChat {
		cp.mu.Unlock()
		return errors.New("session is not a chat session")
	}
	conn := cp.agentConns[sess.ServerID]
	if conn == nil {
		cp.mu.Unlock()
		return errors.New("server offline")
	}

	if sess.Status == SessionStarting {
		sess.Status = SessionRunning
	}
	if inst.Status == SessionStarting {
		inst.Status = SessionRunning
	}

	msgID := uuid.NewString()
	chatMsg := ChatMessage{
		MessageID:  msgID,
		SessionID:  sessionID,
		InstanceID: inst.InstanceID,
		Role:       "user",
		Content:    plainContent,
		Meta:       meta,
		TsMS:       time.Now().UnixMilli(),
	}
	cp.appendChatMessage(sessionID, chatMsg)
	cp.mu.Unlock()

	userBroadcast := NewEnvelope("chat_msg", sess.ServerID, sessionID)
	userBroadcast.InstanceID = inst.InstanceID
	userBroadcast.Data, _ = json.Marshal(chatMsg)
	cp.broadcastToAttached(sessionID, userBroadcast)

	payload, _ := json.Marshal(map[string]any{
		"message_id":    msgID,
		"content":       plainContent,
		"content_parts": parts,
	})
	agentMsg := NewEnvelope("chat_in", sess.ServerID, sessionID)
	agentMsg.InstanceID = inst.InstanceID
	agentMsg.Data = payload
	if err := conn.Send(agentMsg); err != nil {
		return err
	}
	cp.audit.Log(AuditEvent{
		Actor:     actor,
		ServerID:  sess.ServerID,
		SessionID: sessionID,
		Kind:      "chat_in",
		Meta: map[string]any{
			"message_id": msgID,
			"length":     len(plainContent),
			"parts":      len(parts),
		},
	})
	return nil
}

func (cp *ControlPlane) HandleChatOut(serverID, sessionID, instanceID string, data json.RawMessage) {
	var payload struct {
		MessageID string          `json:"message_id"`
		Content   string          `json:"content"`
		Meta      json.RawMessage `json:"meta,omitempty"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return
	}
	var active bool
	cp.mu.Lock()
	sess, ok := cp.sessions[sessionID]
	if !ok {
		cp.mu.Unlock()
		return
	}
	inst := cp.instances[instanceID]
	if inst == nil || inst.SessionID != sessionID {
		cp.mu.Unlock()
		return
	}
	chatMsg := ChatMessage{
		MessageID:  payload.MessageID,
		SessionID:  sessionID,
		InstanceID: instanceID,
		Role:       "assistant",
		Content:    payload.Content,
		Meta:       payload.Meta,
		TsMS:       time.Now().UnixMilli(),
	}
	cp.appendChatMessage(sessionID, chatMsg)
	active = sess.ActiveInstanceID == instanceID
	cp.mu.Unlock()

	if !active {
		return
	}

	broadcast := NewEnvelope("chat_msg", serverID, sessionID)
	broadcast.InstanceID = instanceID
	broadcast.Data, _ = json.Marshal(chatMsg)
	cp.broadcastToAttached(sessionID, broadcast)

	cp.audit.Log(AuditEvent{
		Actor:     "agent:" + serverID,
		ServerID:  serverID,
		SessionID: sessionID,
		Kind:      "chat_out",
		Meta: map[string]any{
			"instance_id": instanceID,
			"message_id":  payload.MessageID,
			"length":      len(payload.Content),
		},
	})
}

func (cp *ControlPlane) HandleChatExit(serverID, sessionID, instanceID string, data json.RawMessage) {
	var exit PTYExit
	_ = json.Unmarshal(data, &exit)
	var active bool
	cp.mu.Lock()
	sess, ok := cp.sessions[sessionID]
	if !ok {
		cp.mu.Unlock()
		return
	}
	inst := cp.instances[instanceID]
	if inst == nil || inst.SessionID != sessionID {
		cp.mu.Unlock()
		return
	}
	inst.Status = SessionExited
	inst.ExitCode = exit.ExitCode
	inst.ExitReason = exit.Reason
	active = sess.ActiveInstanceID == instanceID
	if active {
		sess.Status = SessionExited
		sess.ExitCode = exit.ExitCode
		sess.ExitReason = exit.Reason
	}
	cp.mu.Unlock()

	if !active {
		return
	}
	cp.broadcastSessionUpdate(sessionID)
	cp.audit.Log(AuditEvent{
		Actor:     "agent:" + serverID,
		ServerID:  serverID,
		SessionID: sessionID,
		Kind:      "chat_exit",
		Meta: map[string]any{
			"instance_id": instanceID,
			"reason":      exit.Reason,
			"exit_code":   exit.ExitCode,
		},
	})
}

func (cp *ControlPlane) GetChatHistory(tenantID, sessionID string) ([]ChatMessage, error) {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	sess, ok := cp.sessions[sessionID]
	if !ok {
		return nil, errors.New("session not found")
	}
	if tenantID != "" && sess.TenantID != tenantID {
		return nil, errors.New("session not found")
	}
	if sess.SessionType != SessionTypeChat {
		return nil, errors.New("session is not a chat session")
	}
	msgs := cp.chatHistory[sessionID]
	out := make([]ChatMessage, len(msgs))
	copy(out, msgs)
	return out, nil
}

func (cp *ControlPlane) appendChatMessage(sessionID string, msg ChatMessage) {
	msgs := cp.chatHistory[sessionID]
	msgs = append(msgs, msg)
	if len(msgs) > MaxChatHistory {
		msgs = msgs[len(msgs)-MaxChatHistory:]
	}
	cp.chatHistory[sessionID] = msgs
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

func (cp *ControlPlane) broadcastToTenant(tenantID string, msg Envelope) {
	cp.mu.RLock()
	subs := make([]*Subscriber, 0, len(cp.subscribers))
	for s := range cp.subscribers {
		if tenantID != "" && s.TenantID != tenantID {
			continue
		}
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
		"session_id":         sess.SessionID,
		"instance_id":        sess.ActiveInstanceID,
		"active_instance_id": sess.ActiveInstanceID,
		"session_type":       sess.SessionType,
		"status":             sess.Status,
		"exit_code":          sess.ExitCode,
		"exit_reason":        sess.ExitReason,
		"awaiting_approval":  sess.AwaitingApproval,
		"pending_event_id":   sess.PendingEventID,
	})
	cp.mu.RUnlock()

	msg := NewEnvelope("session_update", serverID, sessionID)
	msg.Data = body
	cp.broadcastToAll(msg)
}
