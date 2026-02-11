package ws

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"cc-control/internal/core"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type ClientHandler struct {
	CP       *core.ControlPlane
	Upgrader websocket.Upgrader
	UIToken  string
}

func (h *ClientHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	remote := r.RemoteAddr
	token := extractToken(r)
	if token == "" || token != h.UIToken || !h.CP.RateAllow("ui:"+token) {
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
		ID:    uuid.NewString(),
		Actor: "ui:" + token,
		Send:  make(chan core.Envelope, 256),
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

	// Debug probe: server-initiated message to UI (logged on backend).
	probe := core.NewEnvelope("debug_probe", "", "")
	probe.Data, _ = json.Marshal(map[string]any{"message": "probe"})
	select {
	case sub.Send <- probe:
	default:
		slog.Warn("ui ws probe dropped", "remote", remote)
	}

	// Replay all unresolved approval events on connect so UI has a global
	// pending-approvals view without requiring per-session attach clicks.
	pendingEvents := h.CP.GetPendingApprovalEvents()
	for _, ev := range pendingEvents {
		evMsg := core.NewEnvelope("event", ev.ServerID, ev.SessionID)
		evMsg.Data, _ = json.Marshal(ev)
		select {
		case sub.Send <- evMsg:
		default:
		}
	}
	if len(pendingEvents) > 0 {
		slog.Info("ui ws replay pending approvals", "remote", remote, "count", len(pendingEvents))
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
			events := h.CP.GetSessionEvents(req.SessionID)
			pendingApprovals := 0
			for _, ev := range events {
				if ev.Kind != "approval_needed" || ev.Resolved {
					continue
				}
				pendingApprovals++
				evMsg := core.NewEnvelope("event", ev.ServerID, ev.SessionID)
				evMsg.Data, _ = json.Marshal(ev)
				select {
				case sub.Send <- evMsg:
				default:
				}
			}
			slog.Info("ui attach", "remote", remote, "session_id", req.SessionID, "pending_approvals", pendingApprovals, "total_events", len(events))
		case "term_in":
			sessionID := msg.SessionID
			if sessionID == "" {
				sessionID = sub.AttachedSession
			}
			if sessionID == "" {
				sub.Send <- errorEnvelope("no_attached_session", "")
				continue
			}
			if err := h.CP.HandleClientTermIn(sub.Actor, sessionID, msg.DataB64); err != nil {
				sub.Send <- errorEnvelope(err.Error(), sessionID)
			}
		case "action":
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
			if err := h.CP.HandleClientAction(sub.Actor, sessionID, req); err != nil {
				sub.Send <- errorEnvelope(err.Error(), sessionID)
			}
		case "resize":
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
			if err := h.CP.HandleClientResize(sub.Actor, sessionID, req.Cols, req.Rows); err != nil {
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
