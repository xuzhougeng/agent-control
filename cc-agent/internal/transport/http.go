package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"cc-agent/internal/agent"
)

// HTTPServer exposes a minimal REST API for programmatic ops:
//   POST /run    {"session_id":"...","input":"..."} -> streams JSON events
type HTTPServer struct {
	addr  string
	agent *agent.Agent
}

func NewHTTPServer(addr string, ag *agent.Agent) *HTTPServer {
	return &HTTPServer{addr: addr, agent: ag}
}

func (h *HTTPServer) ListenAndServe(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/run", h.handleRun)

	srv := &http.Server{Addr: h.addr, Handler: mux}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	return srv.ListenAndServe()
}

type runReq struct {
	SessionID string `json:"session_id"`
	Input     string `json:"input"`
}

func (h *HTTPServer) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req runReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.SessionID == "" || req.Input == "" {
		http.Error(w, "session_id and input required", http.StatusBadRequest)
		return
	}
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "application/x-ndjson")

	prev := h.agent
	prev.SetListener(func(e agent.Event) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"kind":       e.Kind,
			"text":       e.Text,
			"tool_name":  e.ToolName,
			"tool_id":    e.ToolID,
			"tool_input": e.ToolInput,
			"is_error":   e.IsError,
		})
		if flusher != nil {
			flusher.Flush()
		}
	})

	if _, err := h.agent.Run(r.Context(), req.SessionID, req.Input); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"kind": "error", "text": fmt.Sprintf("run: %v", err),
		})
	}
}
