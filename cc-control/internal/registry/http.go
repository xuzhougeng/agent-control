package registry

import (
	"encoding/json"
	"net/http"
)

type IdentityProvider interface {
	ResolveActor(*http.Request) (Actor, error)
}

type RouteDeps struct {
	Store      *Store
	KnownTools []string
	Identity   IdentityProvider
}

// RegisterRoutes wires registry HTTP handlers onto mux. Caller is responsible
// for the path prefix; routes are registered under /api/registry/.
func RegisterRoutes(mux *http.ServeMux, d *RouteDeps) {
	mux.HandleFunc("/api/registry/skills", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			d.handlePublish(w, r)
		case http.MethodGet:
			http.Error(w, "list not implemented", http.StatusNotImplemented)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func (d *RouteDeps) handlePublish(w http.ResponseWriter, r *http.Request) {
	actor, err := d.Identity.ResolveActor(r)
	if err != nil {
		writeJSON(w, 401, errBody{Code: "unauth"})
		return
	}
	var sk Skill
	if err := json.NewDecoder(r.Body).Decode(&sk); err != nil {
		writeJSON(w, 400, errBody{Code: "bad_json", Reason: err.Error()})
		return
	}
	if err := Validate(&sk, d.KnownTools); err != nil {
		ve, _ := err.(*ValidationError)
		writeJSON(w, 400, errBody{Code: "invalid_skill", Field: ve.Field, Reason: ve.Reason})
		return
	}
	authorID := actor.ID
	v, err := d.Store.Publish(&sk, authorID)
	if err != nil {
		writeJSON(w, 500, errBody{Code: "store", Reason: err.Error()})
		return
	}
	writeJSON(w, 201, map[string]any{"name": sk.Name, "version": v, "author_server_id": authorID})
}

type errBody struct {
	Code   string `json:"code"`
	Field  string `json:"field,omitempty"`
	Reason string `json:"reason,omitempty"`
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
