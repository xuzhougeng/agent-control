package ws

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"cc-control/internal/core"
	"github.com/gorilla/websocket"
)

type AgentConn struct {
	conn   *websocket.Conn
	send   chan core.Envelope
	closed chan struct{}
	once   sync.Once
}

func NewAgentConn(conn *websocket.Conn) *AgentConn {
	return &AgentConn{
		conn:   conn,
		send:   make(chan core.Envelope, 128),
		closed: make(chan struct{}),
	}
}

func (a *AgentConn) Send(msg core.Envelope) error {
	select {
	case <-a.closed:
		return websocket.ErrCloseSent
	case a.send <- msg:
		return nil
	}
}

func (a *AgentConn) Close() {
	a.once.Do(func() {
		close(a.closed)
		_ = a.conn.Close()
	})
}

func (a *AgentConn) writeLoop() {
	for {
		select {
		case <-a.closed:
			return
		case msg := <-a.send:
			_ = a.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := a.conn.WriteJSON(msg); err != nil {
				a.Close()
				return
			}
		}
	}
}

type AgentHandler struct {
	CP         *core.ControlPlane
	Upgrader   websocket.Upgrader
	AgentToken string
}

func (h *AgentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)
	if token == "" || token != h.AgentToken || !h.CP.RateAllow("agent:"+token) {
		slog.Warn("agent ws unauthorized", "remote", r.RemoteAddr)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := h.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("agent ws upgrade failed", "remote", r.RemoteAddr, "err", err)
		return
	}
	defer conn.Close()
	slog.Info("agent ws connected", "remote", r.RemoteAddr)

	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		return nil
	})

	// First frame must be register.
	var first core.Envelope
	if err := conn.ReadJSON(&first); err != nil || first.Type != "register" {
		slog.Warn("agent ws missing register", "remote", r.RemoteAddr, "err", err, "type", first.Type)
		return
	}
	var reg core.AgentRegister
	if err := json.Unmarshal(first.Data, &reg); err != nil || reg.ServerID == "" {
		slog.Warn("agent ws bad register", "remote", r.RemoteAddr, "err", err, "server_id", reg.ServerID)
		return
	}

	agentConn := NewAgentConn(conn)
	go agentConn.writeLoop()
	defer agentConn.Close()

	h.CP.RegisterOrUpdateServer(reg, agentConn)
	defer h.CP.RemoveAgentConnection(reg.ServerID)
	slog.Info("agent registered",
		"server_id", reg.ServerID,
		"hostname", reg.Hostname,
		"remote", r.RemoteAddr,
		"tags", reg.Tags,
	)

	ack := core.NewEnvelope("register_ok", reg.ServerID, "")
	ack.Data, _ = json.Marshal(map[string]any{
		"heartbeat_interval_ms": 5000,
		"server_time_ms":        time.Now().UnixMilli(),
	})
	_ = agentConn.Send(ack)
	slog.Info("agent register_ok sent", "server_id", reg.ServerID)

	for {
		var msg core.Envelope
		_ = conn.SetReadDeadline(time.Now().Add(45 * time.Second))
		if err := conn.ReadJSON(&msg); err != nil {
			slog.Warn("agent ws disconnected", "server_id", reg.ServerID, "remote", r.RemoteAddr, "err", err)
			return
		}
		switch msg.Type {
		case "heartbeat":
			h.CP.TouchServer(reg.ServerID)
		case "pty_out":
			h.CP.HandlePTYOut(reg.ServerID, msg.SessionID, msg.Seq, msg.DataB64)
		case "pty_exit":
			var exit core.PTYExit
			_ = json.Unmarshal(msg.Data, &exit)
			h.CP.HandlePTYExit(reg.ServerID, msg.SessionID, exit)
		case "error":
			var payload struct {
				Message string `json:"message"`
			}
			_ = json.Unmarshal(msg.Data, &payload)
			message := payload.Message
			if message == "" {
				message = string(msg.Data)
			}
			h.CP.HandleAgentError(reg.ServerID, msg.SessionID, message)
			slog.Warn("agent error", "server_id", reg.ServerID, "session_id", msg.SessionID, "message", message)
		default:
		}
	}
}
