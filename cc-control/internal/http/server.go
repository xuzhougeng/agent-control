package httpapi

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"cc-control/internal/auth"
	"cc-control/internal/core"
	wshandler "cc-control/internal/ws"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Server struct {
	CP          *core.ControlPlane
	Tokens      *auth.Store
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
		CP:       s.CP,
		Upgrader: upgrader,
		Tokens:   s.Tokens,
	})
	mux.Handle("/ws/client", &wshandler.ClientHandler{
		CP:       s.CP,
		Upgrader: upgrader,
		Tokens:   s.Tokens,
	})

	mux.HandleFunc("/api/servers", s.withUIAuth(s.handleServers))
	mux.HandleFunc("/api/sessions", s.withUIAuth(s.handleSessions))
	mux.HandleFunc("/api/sessions/", s.withUIAuth(s.handleSessionSubroutes))
	mux.HandleFunc("/admin/tokens", s.withAdminAuth(s.handleAdminTokens))
	mux.HandleFunc("/admin/tokens/", s.withAdminAuth(s.handleAdminTokenSubroutes))
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

type authedHandler func(http.ResponseWriter, *http.Request, *auth.TokenRecord)

func (s *Server) withUIAuth(next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)
		if token == "" || s.Tokens == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		rec, ok := s.Tokens.Lookup(token)
		if !ok || rec.Revoked || rec.Type != auth.TokenTypeUI || !s.CP.RateAllow("ui:"+rec.TokenID) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r, rec)
	}
}

func (s *Server) withAdminAuth(next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)
		if token == "" || s.Tokens == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		rec, ok := s.Tokens.Lookup(token)
		if !ok || rec.Revoked || rec.Type != auth.TokenTypeAdmin {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r, rec)
	}
}

func (s *Server) handleServers(w http.ResponseWriter, r *http.Request, rec *auth.TokenRecord) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !auth.RoleAtLeast(rec.Role, auth.RoleViewer) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": s.CP.GetServers(rec.TenantID)})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request, rec *auth.TokenRecord) {
	switch r.Method {
	case http.MethodGet:
		if !auth.RoleAtLeast(rec.Role, auth.RoleViewer) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		serverID := r.URL.Query().Get("server_id")
		writeJSON(w, http.StatusOK, map[string]any{"sessions": s.CP.GetSessions(rec.TenantID, serverID)})
	case http.MethodPost:
		if !auth.RoleAtLeast(rec.Role, auth.RoleOperator) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var req core.StartSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		actor := "ui:" + rec.TokenID
		sess, err := s.CP.CreateSession(actor, rec.TenantID, req)
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

func (s *Server) handleSessionSubroutes(w http.ResponseWriter, r *http.Request, rec *auth.TokenRecord) {
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
		if !auth.RoleAtLeast(rec.Role, auth.RoleOperator) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var req core.StopSessionRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		actor := "ui:" + rec.TokenID
		if err := s.CP.StopSession(actor, rec.TenantID, sessionID, req.GraceMS, req.KillAfterMS); err != nil {
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
		if !auth.RoleAtLeast(rec.Role, auth.RoleOwner) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var req core.StopSessionRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		actor := "ui:" + rec.TokenID
		if err := s.CP.StopAndDeleteSession(actor, rec.TenantID, sessionID, req.GraceMS, req.KillAfterMS); err != nil {
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
		if !auth.RoleAtLeast(rec.Role, auth.RoleViewer) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		events := s.CP.GetSessionEvents(rec.TenantID, sessionID)
		writeJSON(w, http.StatusOK, map[string]any{"events": events})
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) handleAdminTokens(w http.ResponseWriter, r *http.Request, _ *auth.TokenRecord) {
	switch r.Method {
	case http.MethodGet:
		tenantID := strings.TrimSpace(r.URL.Query().Get("tenant_id"))
		writeJSON(w, http.StatusOK, map[string]any{"tokens": s.Tokens.ListTokens(tenantID)})
	case http.MethodPost:
		var req struct {
			Type     string `json:"type"`
			TenantID string `json:"tenant_id"`
			Role     string `json:"role"`
			Name     string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		tt, ok := auth.ParseType(strings.TrimSpace(req.Type))
		if !ok || tt == auth.TokenTypeAdmin {
			http.Error(w, "invalid token type", http.StatusBadRequest)
			return
		}
		if req.TenantID == "" {
			req.TenantID = uuid.NewString()
		}
		role := auth.TokenRole("")
		if tt == auth.TokenTypeUI {
			var roleOK bool
			role, roleOK = auth.ParseRole(strings.TrimSpace(req.Role))
			if !roleOK {
				http.Error(w, "invalid role", http.StatusBadRequest)
				return
			}
		}
		plain, rec, err := s.Tokens.CreateToken(tt, role, req.TenantID, strings.TrimSpace(req.Name))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"token":         plain,
			"token_id":      rec.TokenID,
			"tenant_id":     rec.TenantID,
			"type":          rec.Type,
			"role":          rec.Role,
			"created_at_ms": rec.CreatedAtMS,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAdminTokenSubroutes(w http.ResponseWriter, r *http.Request, _ *auth.TokenRecord) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/tokens/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] != "revoke" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tokenID := parts[0]
	if ok := s.Tokens.RevokeToken(tokenID); !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
