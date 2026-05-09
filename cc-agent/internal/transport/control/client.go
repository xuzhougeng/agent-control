package control

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"cc-agent/internal/agent"
	"cc-agent/internal/skills"
)

// installedSkillNameRE constrains skill names accepted from install_skill_request
// envelopes. Conservative subset of POSIX path-portable characters that also
// avoids any traversal sequences (".."/"/"). Mirrors the registry's intent.
var installedSkillNameRE = regexp.MustCompile(`^[a-z][a-z0-9-]{1,63}$`)

// Options configures a Client. Agent and ServerID are required; everything
// else has sensible defaults.
type Options struct {
	URL            string
	Token          string
	ServerID       string
	Tags           []string
	AllowedRoots   []string
	Hostname       string
	HeartbeatEvery time.Duration
	TLSSkipVerify  bool
	AgentVersion   string
	// TeamDir is where skills delivered via install_skill_request envelopes
	// are written. Empty disables install_skill_request handling.
	TeamDir string

	Agent *agent.Agent
}

// Client owns the WS connection to cc-control and dispatches inbound chat
// envelopes to the local agent. One Client per cc-agent process.
//
// When an Approver is attached the client also routes inbound
// approval_decision envelopes back to the awaiting tool call.
type Client struct {
	opts Options

	mu        sync.Mutex
	instances map[string]*chatInstance
	send      chan Envelope
	approver  *RemoteApprover
}

// SetApprover attaches a RemoteApprover so this client can route
// approval_decision envelopes back into pending bash approvals. Call before
// Run.
func (c *Client) SetApprover(a *RemoteApprover) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.approver = a
}

// chatInstance tracks one logical chat session lifetime. session_id and
// instance_id come from the control plane; cancel ends an in-flight Run.
type chatInstance struct {
	sessionID  string
	instanceID string
	cwd        string
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

var errVersionMismatch = errors.New("protocol version mismatch")

func New(opts Options) (*Client, error) {
	if opts.Agent == nil {
		return nil, errors.New("control: agent required")
	}
	if opts.ServerID == "" {
		return nil, errors.New("control: server_id required")
	}
	if opts.Token == "" {
		return nil, errors.New("control: agent token required")
	}
	url, err := normalizeWSURL(opts.URL)
	if err != nil {
		return nil, fmt.Errorf("control: bad url: %w", err)
	}
	opts.URL = url
	if opts.HeartbeatEvery <= 0 {
		opts.HeartbeatEvery = 5 * time.Second
	}
	if opts.Hostname == "" {
		opts.Hostname, _ = os.Hostname()
	}
	if opts.AgentVersion == "" {
		opts.AgentVersion = "cc-agent dev"
	}
	return &Client{
		opts:      opts,
		instances: map[string]*chatInstance{},
	}, nil
}

// Run drives the connection lifecycle: dial → register → message loop →
// reconnect with backoff. Returns when ctx is canceled.
func (c *Client) Run(ctx context.Context) error {
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		err := c.runOnce(ctx)
		if err != nil {
			if errors.Is(err, errVersionMismatch) {
				slog.Error("cc-control protocol mismatch; pausing 5 minutes")
				backoff = 5 * time.Minute
			} else {
				slog.Warn("cc-control disconnected", "err", err)
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff < 8*time.Second {
			backoff *= 2
		}
	}
}

func (c *Client) runOnce(ctx context.Context) error {
	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.opts.Token)
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	if c.opts.TLSSkipVerify {
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	conn, _, err := dialer.DialContext(ctx, c.opts.URL, header)
	if err != nil {
		return err
	}
	defer conn.Close()
	slog.Info("cc-control connected", "url", c.opts.URL, "server_id", c.opts.ServerID)

	c.mu.Lock()
	c.send = make(chan Envelope, 256)
	c.mu.Unlock()

	connCtx, cancelConn := context.WithCancel(ctx)
	defer cancelConn()
	defer c.cancelAllInstances()

	var writeMu sync.Mutex
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for {
			select {
			case <-connCtx.Done():
				return
			case msg := <-c.send:
				writeMu.Lock()
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				err := conn.WriteJSON(msg)
				writeMu.Unlock()
				if err != nil {
					return
				}
			}
		}
	}()

	regData, _ := json.Marshal(c.registerPayload())
	reg := newEnvelope("register", c.opts.ServerID, "")
	reg.Data = regData
	if err := c.enqueue(reg); err != nil {
		cancelConn()
		<-writerDone
		return err
	}

	hbTicker := time.NewTicker(c.opts.HeartbeatEvery)
	defer hbTicker.Stop()
	go func() {
		for {
			select {
			case <-connCtx.Done():
				return
			case <-hbTicker.C:
				_ = c.enqueue(newEnvelope("heartbeat", c.opts.ServerID, ""))
			}
		}
	}()

	for {
		var msg Envelope
		if err := conn.ReadJSON(&msg); err != nil {
			cancelConn()
			<-writerDone
			return err
		}
		if err := c.handle(connCtx, msg); err != nil {
			if errors.Is(err, errVersionMismatch) {
				cancelConn()
				<-writerDone
				return err
			}
			c.sendError(msg.SessionID, msg.InstanceID, err.Error())
		}
	}
}

func (c *Client) registerPayload() RegisterPayload {
	return RegisterPayload{
		ServerID:        c.opts.ServerID,
		Hostname:        c.opts.Hostname,
		Tags:            append([]string(nil), c.opts.Tags...),
		OS:              runtime.GOOS,
		Arch:            runtime.GOARCH,
		AgentVersion:    c.opts.AgentVersion,
		ProtocolVersion: ProtocolVersion,
		AllowRoots:      append([]string(nil), c.opts.AllowedRoots...),
		ClaudePath:      "cc-agent-builtin",
	}
}

func (c *Client) handle(ctx context.Context, msg Envelope) error {
	switch msg.Type {
	case "register_ok":
		slog.Info("cc-control register_ok", "server_id", c.opts.ServerID)
		return nil
	case "version_error":
		var payload struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(msg.Data, &payload)
		slog.Error("cc-control version mismatch", "msg", payload.Message)
		return errVersionMismatch
	case "session_update", "event":
		return nil
	case "start_chat":
		var p StartChatPayload
		if err := json.Unmarshal(msg.Data, &p); err != nil {
			return fmt.Errorf("start_chat decode: %w", err)
		}
		return c.startChat(ctx, msg.SessionID, msg.InstanceID, p)
	case "chat_in":
		var p ChatInPayload
		if err := json.Unmarshal(msg.Data, &p); err != nil {
			slog.Error("chat_in decode failed", "err", err, "raw", string(msg.Data))
			return fmt.Errorf("chat_in decode: %w", err)
		}
		slog.Info("chat_in received", "session", msg.SessionID, "instance", msg.InstanceID, "message_id", p.MessageID, "len", len(p.Content))
		return c.chatIn(msg.SessionID, msg.InstanceID, p)
	case "stop_session":
		return c.stopChat(msg.SessionID, msg.InstanceID, "operator stopped")
	case "approval_decision":
		var p ApprovalDecisionPayload
		if err := json.Unmarshal(msg.Data, &p); err != nil {
			return fmt.Errorf("approval_decision decode: %w", err)
		}
		c.mu.Lock()
		approver := c.approver
		c.mu.Unlock()
		hasApprover := approver != nil
		var pendingFound bool
		if approver != nil {
			approver.mu.Lock()
			_, pendingFound = approver.pending[p.RequestID]
			approver.mu.Unlock()
			approver.dispatch(p)
		}
		slog.Info("approval_decision received",
			"request_id", p.RequestID, "approved", p.Approved, "actor", p.Actor,
			"approver_attached", hasApprover, "pending_found", pendingFound)
		return nil
	case "install_skill_request":
		return c.installSkillRequest(msg)
	case "start_session":
		// Terminal sessions are cc-proxy's job; explicitly reject so the
		// operator gets a clear message instead of a silent drop.
		return errors.New("cc-agent only supports chat mode; use cc-proxy for PTY")
	case "heartbeat":
		return nil
	default:
		return nil
	}
}

func (c *Client) startChat(ctx context.Context, sessionID, instanceID string, p StartChatPayload) error {
	if sessionID == "" || instanceID == "" {
		return errors.New("missing session_id or instance_id")
	}
	if err := validateCwd(p.Cwd, c.opts.AllowedRoots); err != nil {
		return fmt.Errorf("reject_cwd: %w", err)
	}

	c.mu.Lock()
	if _, ok := c.instances[instanceID]; ok {
		c.mu.Unlock()
		return errors.New("chat instance already exists")
	}
	instCtx, cancel := context.WithCancel(ctx)
	c.instances[instanceID] = &chatInstance{
		sessionID: sessionID, instanceID: instanceID, cwd: p.Cwd, cancel: cancel,
	}
	c.mu.Unlock()

	// The agent loop is shared across instances; we don't spawn anything per
	// chat. Just track the instance and acknowledge.
	_ = instCtx
	slog.Info("chat session started", "session", sessionID, "instance", instanceID, "cwd", p.Cwd)
	return nil
}

func (c *Client) chatIn(sessionID, instanceID string, p ChatInPayload) error {
	c.mu.Lock()
	inst, ok := c.instances[instanceID]
	if !ok {
		c.mu.Unlock()
		return errors.New("chat instance not found")
	}
	c.mu.Unlock()

	if p.Content == "" && len(p.ContentParts) > 0 {
		p.Content = flattenParts(p.ContentParts)
	}
	if p.Content == "" {
		return errors.New("empty chat content")
	}

	inst.wg.Add(1)
	go func() {
		defer inst.wg.Done()
		c.runAgentTurn(inst, p)
	}()
	return nil
}

func (c *Client) runAgentTurn(inst *chatInstance, p ChatInPayload) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Stamp the session+instance into the ctx so RemoteApprover (called by
	// the bash tool deep inside the agent loop) can address its
	// approval_request to the right session in cc-control.
	ctx = WithSession(ctx, inst.sessionID, inst.instanceID)

	// Each tool_use / tool_result emits a progress chat_out so the UI shows a
	// transient bubble immediately instead of waiting 30s-2min for the final
	// answer. Operations are also accumulated so the final message keeps a
	// summary in meta.operations.
	var (
		opsMu sync.Mutex
		ops   []string
		seq   int
	)
	nextID := func() string {
		seq++
		return fmt.Sprintf("%s/%d", p.MessageID, seq)
	}
	listener := func(e agent.Event) {
		switch e.Kind {
		case agent.EventToolCall:
			step := fmt.Sprintf("tool_use: %s %s", e.ToolName, summarizeInput(e.ToolInput))
			opsMu.Lock()
			ops = append(ops, step)
			opsMu.Unlock()
			c.sendChatProgress(inst.sessionID, inst.instanceID, nextID(),
				fmt.Sprintf("▶ %s %s", e.ToolName, summarizeInput(e.ToolInput)))
		case agent.EventToolResult:
			snippet := e.Text
			if len(snippet) > 160 {
				snippet = snippet[:160] + "…"
			}
			label := "result"
			if e.IsError {
				label = "error"
			}
			step := fmt.Sprintf("%s: %s", label, strings.ReplaceAll(snippet, "\n", " "))
			opsMu.Lock()
			ops = append(ops, step)
			opsMu.Unlock()
			prefix := "✓"
			if e.IsError {
				prefix = "✗"
			}
			c.sendChatProgress(inst.sessionID, inst.instanceID, nextID(),
				fmt.Sprintf("%s %s", prefix, clipForBubble(snippet)))
		}
	}

	slog.Info("agent turn start", "session", inst.sessionID, "message_id", p.MessageID)
	final, runErr := c.opts.Agent.RunWithListener(ctx, inst.sessionID, p.Content, listener)
	if runErr != nil {
		slog.Error("agent turn failed", "session", inst.sessionID, "message_id", p.MessageID, "err", runErr)
		c.sendChatFinal(inst.sessionID, inst.instanceID, nextID(), fmt.Sprintf("agent error: %v", runErr), ops)
		return
	}
	slog.Info("agent turn done", "session", inst.sessionID, "message_id", p.MessageID, "ops", len(ops), "final_len", len(final))
	c.sendChatFinal(inst.sessionID, inst.instanceID, nextID(), final, ops)
}

func clipForBubble(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

func (c *Client) stopChat(sessionID, instanceID, reason string) error {
	c.mu.Lock()
	inst, ok := c.instances[instanceID]
	if ok {
		delete(c.instances, instanceID)
	}
	c.mu.Unlock()
	if !ok {
		return nil
	}
	inst.cancel()
	go func() {
		inst.wg.Wait()
		zero := 0
		c.sendChatExit(sessionID, instanceID, &zero, reason)
	}()
	return nil
}

func (c *Client) cancelAllInstances() {
	c.mu.Lock()
	insts := c.instances
	c.instances = map[string]*chatInstance{}
	c.mu.Unlock()
	for _, inst := range insts {
		inst.cancel()
	}
}

func (c *Client) sendChatProgress(sessionID, instanceID, messageID, content string) {
	c.sendChatOutWithMeta(sessionID, instanceID, messageID, content, ChatOutMeta{
		Source: "claude-stream-json", Progress: true, TsMS: time.Now().UnixMilli(),
	})
}

func (c *Client) sendChatFinal(sessionID, instanceID, messageID, content string, operations []string) {
	c.sendChatOutWithMeta(sessionID, instanceID, messageID, content, ChatOutMeta{
		Source: "claude-stream-json", Final: true, Operations: operations, TsMS: time.Now().UnixMilli(),
	})
}

func (c *Client) sendChatOutWithMeta(sessionID, instanceID, messageID, content string, m ChatOutMeta) {
	meta, _ := json.Marshal(m)
	payload, _ := json.Marshal(ChatOutPayload{
		MessageID: messageID, Content: content, Meta: meta,
	})
	env := newEnvelope("chat_out", c.opts.ServerID, sessionID)
	env.InstanceID = instanceID
	env.Data = payload
	_ = c.enqueue(env)
}

func (c *Client) sendChatExit(sessionID, instanceID string, exitCode *int, reason string) {
	payload, _ := json.Marshal(ChatExitPayload{ExitCode: exitCode, Reason: reason})
	env := newEnvelope("chat_exit", c.opts.ServerID, sessionID)
	env.InstanceID = instanceID
	env.Data = payload
	_ = c.enqueue(env)
}

func (c *Client) sendError(sessionID, instanceID, message string) {
	payload, _ := json.Marshal(map[string]string{"message": message})
	env := newEnvelope("error", c.opts.ServerID, sessionID)
	env.InstanceID = instanceID
	env.Data = payload
	_ = c.enqueue(env)
}

func (c *Client) enqueue(env Envelope) error {
	c.mu.Lock()
	send := c.send
	c.mu.Unlock()
	if send == nil {
		return errors.New("not connected")
	}
	select {
	case send <- env:
		return nil
	default:
		return errors.New("send queue full")
	}
}

// writeInstalledSkill writes a registry-delivered skill to teamDir atomically.
// Mirrors skills.RegistryClient.Install's file-write semantics so the on-disk
// layout of UI-installed and CLI-installed skills is identical.
func writeInstalledSkill(teamDir string, sk *skills.StoredSkill) error {
	if teamDir == "" {
		return errors.New("install_skill_request: TeamDir not configured")
	}
	if sk == nil || sk.Name == "" {
		return errors.New("install_skill_request: skill missing name")
	}
	// Name validation is the only barrier between an attacker-controlled
	// envelope and arbitrary file writes; reject anything that isn't a plain
	// lowercase identifier.
	if !installedSkillNameRE.MatchString(sk.Name) {
		return fmt.Errorf("install_skill_request: invalid name %q", sk.Name)
	}
	if err := os.MkdirAll(teamDir, 0o755); err != nil {
		return fmt.Errorf("mkdir team: %w", err)
	}
	final := filepath.Join(teamDir, sk.Name+".json")
	tmp := final + ".tmp"
	body, err := json.MarshalIndent(sk.Skill, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// installSkillRequest handles an install_skill_request envelope from
// cc-control: decodes the StoredSkill payload, writes it to TeamDir, and
// reloads the agent's skill registry so the new skill is immediately
// available without a restart.
func (c *Client) installSkillRequest(msg Envelope) error {
	var sk skills.StoredSkill
	if err := json.Unmarshal(msg.Data, &sk); err != nil {
		slog.Error("install_skill_request decode failed", "err", err)
		return fmt.Errorf("install_skill_request decode: %w", err)
	}
	if err := writeInstalledSkill(c.opts.TeamDir, &sk); err != nil {
		slog.Error("install_skill_request write failed", "err", err, "name", sk.Name)
		return err
	}
	if c.opts.Agent != nil {
		if reg := c.opts.Agent.Skills(); reg != nil {
			if err := reg.LoadDir(c.opts.TeamDir); err != nil {
				slog.Warn("install_skill_request reload failed", "err", err)
			}
		}
	}
	slog.Info("install_skill_request applied", "name", sk.Name, "version", sk.Version)
	return nil
}

func validateCwd(cwd string, allowedRoots []string) error {
	if cwd == "" {
		return errors.New("cwd is empty")
	}
	if len(allowedRoots) == 0 {
		return nil // no containment configured
	}
	for _, root := range allowedRoots {
		if root == "" {
			continue
		}
		if cwd == root || strings.HasPrefix(cwd, strings.TrimRight(root, "/")+"/") {
			return nil
		}
	}
	return fmt.Errorf("cwd %q not under any allowed_root", cwd)
}

func flattenParts(parts []ChatContentPart) string {
	var b strings.Builder
	for _, p := range parts {
		if p.Type == "text" && p.Text != "" {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

func summarizeInput(in map[string]any) string {
	if len(in) == 0 {
		return ""
	}
	parts := make([]string, 0, len(in))
	for k, v := range in {
		s := fmt.Sprintf("%v", v)
		if len(s) > 60 {
			s = s[:60] + "…"
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, s))
	}
	return "{" + strings.Join(parts, " ") + "}"
}

func normalizeWSURL(base string) (string, error) {
	if base == "" {
		return "", errors.New("empty url")
	}
	if strings.HasPrefix(base, "ws://") || strings.HasPrefix(base, "wss://") {
		return base, nil
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}
	return u.String(), nil
}
