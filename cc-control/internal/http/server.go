package httpapi

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"cc-control/internal/core"
	wshandler "cc-control/internal/ws"
	"github.com/gorilla/websocket"
)

type Server struct {
	CP          *core.ControlPlane
	AgentToken  string
	UIToken     string
	UIDir       string
	CheckOrigin bool
}

func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			if s.CheckOrigin {
				return sameHostOrigin(r)
			}
			return true
		},
	}

	mux.Handle("/ws/agent", &wshandler.AgentHandler{
		CP:         s.CP,
		Upgrader:   upgrader,
		AgentToken: s.AgentToken,
	})
	mux.Handle("/ws/client", &wshandler.ClientHandler{
		CP:       s.CP,
		Upgrader: upgrader,
		UIToken:  s.UIToken,
	})

	mux.HandleFunc("/api/servers", s.withUIAuth(s.handleServers))
	mux.HandleFunc("/api/sessions", s.withUIAuth(s.handleSessions))
	mux.HandleFunc("/api/sessions/", s.withUIAuth(s.handleSessionSubroutes))
	mux.HandleFunc("/api/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	uiDir := s.UIDir
	if uiDir == "" {
		uiDir = "ui"
	}
	mux.Handle("/", http.FileServer(http.Dir(filepath.Clean(uiDir))))
	return mux
}

func (s *Server) withUIAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)
		if token == "" || token != s.UIToken || !s.CP.RateAllow("ui:"+token) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func (s *Server) handleServers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": s.CP.GetServers()})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		serverID := r.URL.Query().Get("server_id")
		writeJSON(w, http.StatusOK, map[string]any{"sessions": s.CP.GetSessions(serverID)})
	case http.MethodPost:
		var req core.StartSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		actor := "ui:" + extractToken(r)
		sess, err := s.CP.CreateSession(actor, req)
		if err != nil {
			code := http.StatusInternalServerError
			if strings.Contains(err.Error(), "offline") {
				code = http.StatusServiceUnavailable
			}
			http.Error(w, err.Error(), code)
			return
		}
		writeJSON(w, http.StatusCreated, sess)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSessionSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	sessionID := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch {
	case r.Method == http.MethodPost && action == "stop":
		var req core.StopSessionRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		actor := "ui:" + extractToken(r)
		if err := s.CP.StopSession(actor, sessionID, req.GraceMS, req.KillAfterMS); err != nil {
			code := http.StatusInternalServerError
			if strings.Contains(err.Error(), "not found") {
				code = http.StatusNotFound
			}
			if strings.Contains(err.Error(), "offline") {
				code = http.StatusServiceUnavailable
			}
			http.Error(w, err.Error(), code)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case r.Method == http.MethodDelete && action == "":
		var req core.StopSessionRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		actor := "ui:" + extractToken(r)
		if err := s.CP.StopAndDeleteSession(actor, sessionID, req.GraceMS, req.KillAfterMS); err != nil {
			code := http.StatusInternalServerError
			if strings.Contains(err.Error(), "not found") {
				code = http.StatusNotFound
			}
			if strings.Contains(err.Error(), "offline") {
				code = http.StatusServiceUnavailable
			}
			http.Error(w, err.Error(), code)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case r.Method == http.MethodGet && action == "events":
		events := s.CP.GetSessionEvents(sessionID)
		writeJSON(w, http.StatusOK, map[string]any{"events": events})
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func extractToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[len("Bearer "):])
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func sameHostOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	return strings.Contains(origin, r.Host)
}
