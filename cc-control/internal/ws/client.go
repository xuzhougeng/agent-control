package ws

import (
	"encoding/base64"
	"encoding/json"
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
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteJSON(msg); err != nil {
					return
				}
			}
		}
	}()

	for {
		var msg core.Envelope
		if err := conn.ReadJSON(&msg); err != nil {
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
