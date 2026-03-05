package ws

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"cc-control/internal/auth"
	"cc-control/internal/core"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type ClientHandler struct {
	CP       *core.ControlPlane
	Upgrader websocket.Upgrader
	Tokens   *auth.Store
}

func (h *ClientHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	remote := r.RemoteAddr
	token := extractToken(r)
	if token == "" || h.Tokens == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	rec, ok := h.Tokens.Lookup(token)
	if !ok || rec.Revoked || rec.Type != auth.TokenTypeUI || !auth.RoleAtLeast(rec.Role, auth.RoleViewer) || !h.CP.RateAllow("ui:"+rec.TokenID) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := h.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	slog.Info("ui ws connected", "remote", remote)

	sub := &core.Subscriber{
		ID:       uuid.NewString(),
		Actor:    "ui:" + rec.TokenID,
		Send:     make(chan core.Envelope, 256),
		TenantID: rec.TenantID,
	}
	h.CP.RegisterSubscriber(sub)
	stopWriter := make(chan struct{})
	var stopOnce sync.Once
	cleanup := func() {
		stopOnce.Do(func() {
			h.CP.UnregisterSubscriber(sub)
			close(stopWriter)
		})
	}
	defer cleanup()

	doneWriter := make(chan struct{})
	go func() {
		defer close(doneWriter)
		for {
			select {
			case <-stopWriter:
				return
			case msg := <-sub.Send:
				if msg.Type != "term_out" {
					slog.Info("ui ws send", "remote", remote, "type", msg.Type, "session_id", msg.SessionID)
				}
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteJSON(msg); err != nil {
					return
				}
			}
		}
	}()

	// Parse client protocol version from URL query parameter.
	clientProtoVersion := 0
	if v := r.URL.Query().Get("v"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			clientProtoVersion = parsed
		}
	}

	// Hello message: send server protocol version to client.
	hello := core.NewEnvelope("hello", "", "")
	hello.Data, _ = json.Marshal(map[string]any{
		"protocol_version":     core.ProtocolVersion,
		"protocol_version_min": core.ProtocolVersionMin,
		"server_time_ms":       time.Now().UnixMilli(),
	})
	select {
	case sub.Send <- hello:
	default:
		slog.Warn("ui ws hello dropped", "remote", remote)
	}

	// Protocol version check.
	if clientProtoVersion > core.ProtocolVersion {
		errMsg := core.NewEnvelope("version_error", "", "")
		errMsg.Data, _ = json.Marshal(map[string]any{
			"message":                 "client protocol version too new; upgrade the control plane",
			"server_protocol_version": core.ProtocolVersion,
		})
		_ = conn.WriteJSON(errMsg)
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "protocol_version_mismatch"),
			time.Now().Add(2*time.Second),
		)
		slog.Warn("ui client protocol version too new",
			"remote", remote,
			"client_protocol_version", clientProtoVersion,
			"server_protocol_version", core.ProtocolVersion,
		)
		cleanup()
		<-doneWriter
		return
	}
	if clientProtoVersion < core.ProtocolVersionMin {
		errMsg := core.NewEnvelope("version_error", "", "")
		errMsg.Data, _ = json.Marshal(map[string]any{
			"message":                 "client version too old; please refresh the page",
			"server_protocol_version": core.ProtocolVersion,
			"min_protocol_version":    core.ProtocolVersionMin,
		})
		_ = conn.WriteJSON(errMsg)
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "protocol_version_too_old"),
			time.Now().Add(2*time.Second),
		)
		slog.Warn("ui client protocol version too old",
			"remote", remote,
			"client_protocol_version", clientProtoVersion,
			"min_protocol_version", core.ProtocolVersionMin,
		)
		cleanup()
		<-doneWriter
		return
	}
	if clientProtoVersion < core.ProtocolVersion {
		slog.Warn("ui client using older protocol version",
			"remote", remote,
			"client_protocol_version", clientProtoVersion,
			"server_protocol_version", core.ProtocolVersion,
		)
	}

	// Replay all unresolved approval events on connect so UI has a global
	// pending-approvals view without requiring per-session attach clicks.
	pendingEvents := h.CP.GetPendingApprovalEvents(rec.TenantID)
	for _, ev := range pendingEvents {
		evMsg := core.NewEnvelope("event", ev.ServerID, ev.SessionID)
		evMsg.InstanceID = ev.InstanceID
		evMsg.Data, _ = json.Marshal(ev)
		select {
		case sub.Send <- evMsg:
		default:
		}
	}
	if len(pendingEvents) > 0 {
		slog.Info("ui ws replay pending approvals", "remote", remote, "count", len(pendingEvents))
	}

	// Replay recent notifications so short reconnect windows do not lose reminders.
	recentNotices := h.CP.GetRecentNotificationEvents(rec.TenantID, 20)
	for _, ev := range recentNotices {
		evMsg := core.NewEnvelope("event", ev.ServerID, ev.SessionID)
		evMsg.Data, _ = json.Marshal(ev)
		select {
		case sub.Send <- evMsg:
		default:
		}
	}
	if len(recentNotices) > 0 {
		slog.Info("ui ws replay notifications", "remote", remote, "count", len(recentNotices))
	}

	for {
		var msg core.Envelope
		if err := conn.ReadJSON(&msg); err != nil {
			slog.Info("ui ws disconnected", "remote", remote, "err", err)
			cleanup()
			<-doneWriter
			return
		}
		switch msg.Type {
		case "attach":
			var req struct {
				SessionID string `json:"session_id"`
				SinceSeq  uint64 `json:"since_seq"`
			}
			if err := json.Unmarshal(msg.Data, &req); err != nil || req.SessionID == "" {
				_ = conn.WriteJSON(errorEnvelope("bad_attach_payload", msg.SessionID))
				continue
			}
			snapshot, latest, err := h.CP.AttachSubscriber(sub, req.SessionID)
			if err != nil {
				_ = conn.WriteJSON(errorEnvelope(err.Error(), req.SessionID))
				continue
			}
			ack := core.NewEnvelope("attach_ok", "", req.SessionID)
			ack.Data, _ = json.Marshal(map[string]any{
				"session_id": req.SessionID,
				"latest_seq": latest,
			})
			sub.Send <- ack
			if len(snapshot) > 0 {
				out := core.NewEnvelope("term_out", "", req.SessionID)
				out.Seq = latest
				out.DataB64 = encodeB64(snapshot)
				sub.Send <- out
			}

			// Re-send pending approval events for this session to recover from transient drops.
			events := h.CP.GetSessionEvents(rec.TenantID, req.SessionID)
			pendingApprovals := 0
			for _, ev := range events {
				if ev.Kind != "approval_needed" || ev.Resolved {
					continue
				}
				pendingApprovals++
				evMsg := core.NewEnvelope("event", ev.ServerID, ev.SessionID)
				evMsg.InstanceID = ev.InstanceID
				evMsg.Data, _ = json.Marshal(ev)
				select {
				case sub.Send <- evMsg:
				default:
				}
			}
			slog.Info("ui attach", "remote", remote, "session_id", req.SessionID, "pending_approvals", pendingApprovals, "total_events", len(events))
		case "term_in":
			if !auth.RoleAtLeast(rec.Role, auth.RoleOperator) {
				sub.Send <- errorEnvelope("forbidden", msg.SessionID)
				continue
			}
			sessionID := msg.SessionID
			if sessionID == "" {
				sessionID = sub.AttachedSession
			}
			if sessionID == "" {
				sub.Send <- errorEnvelope("no_attached_session", "")
				continue
			}
			if err := h.CP.HandleClientTermIn(sub.Actor, rec.TenantID, sessionID, msg.DataB64); err != nil {
				sub.Send <- errorEnvelope(err.Error(), sessionID)
			}
		case "action":
			if !auth.RoleAtLeast(rec.Role, auth.RoleOperator) {
				sub.Send <- errorEnvelope("forbidden", msg.SessionID)
				continue
			}
			var req core.ActionRequest
			if err := json.Unmarshal(msg.Data, &req); err != nil {
				sub.Send <- errorEnvelope("bad_action_payload", msg.SessionID)
				continue
			}
			sessionID := msg.SessionID
			if sessionID == "" {
				sessionID = sub.AttachedSession
			}
			if sessionID == "" {
				sub.Send <- errorEnvelope("no_attached_session", "")
				continue
			}
			if err := h.CP.HandleClientAction(sub.Actor, rec.TenantID, sessionID, req); err != nil {
				sub.Send <- errorEnvelope(err.Error(), sessionID)
			}
		case "resize":
			if !auth.RoleAtLeast(rec.Role, auth.RoleOperator) {
				sub.Send <- errorEnvelope("forbidden", msg.SessionID)
				continue
			}
			var req struct {
				Cols uint16 `json:"cols"`
				Rows uint16 `json:"rows"`
			}
			if err := json.Unmarshal(msg.Data, &req); err != nil {
				sub.Send <- errorEnvelope("bad_resize_payload", msg.SessionID)
				continue
			}
			sessionID := msg.SessionID
			if sessionID == "" {
				sessionID = sub.AttachedSession
			}
			if sessionID == "" {
				sub.Send <- errorEnvelope("no_attached_session", "")
				continue
			}
			if err := h.CP.HandleClientResize(sub.Actor, rec.TenantID, sessionID, req.Cols, req.Rows); err != nil {
				sub.Send <- errorEnvelope(err.Error(), sessionID)
			}
		case "chat_in":
			if !auth.RoleAtLeast(rec.Role, auth.RoleOperator) {
				sub.Send <- errorEnvelope("forbidden", msg.SessionID)
				continue
			}
			var req struct {
				Content      string                 `json:"content"`
				ContentParts []core.ChatContentPart `json:"content_parts"`
			}
			if err := json.Unmarshal(msg.Data, &req); err != nil {
				sub.Send <- errorEnvelope("bad_chat_in_payload", msg.SessionID)
				continue
			}
			if strings.TrimSpace(req.Content) == "" && len(req.ContentParts) == 0 {
				sub.Send <- errorEnvelope("bad_chat_in_payload", msg.SessionID)
				continue
			}
			sessionID := msg.SessionID
			if sessionID == "" {
				sessionID = sub.AttachedSession
			}
			if sessionID == "" {
				sub.Send <- errorEnvelope("no_attached_session", "")
				continue
			}
			if err := h.CP.HandleClientChatIn(sub.Actor, rec.TenantID, sessionID, req.Content, req.ContentParts); err != nil {
				sub.Send <- errorEnvelope(err.Error(), sessionID)
			}
		default:
			sub.Send <- errorEnvelope("unknown_type", msg.SessionID)
		}
	}
}

func errorEnvelope(reason, sessionID string) core.Envelope {
	env := core.NewEnvelope("error", "", sessionID)
	env.Data, _ = json.Marshal(map[string]any{"message": reason})
	return env
}

func encodeB64(p []byte) string {
	if len(p) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(p)
}
