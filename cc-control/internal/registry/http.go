package registry

import (
	"encoding/json"
	"net/http"
	"strconv"
)

type IdentityProvider interface {
	ResolveActor(*http.Request) (Actor, error)
}

// InstallNotifier delivers an install_skill_request to a connected agent over
// whatever transport cc-control already uses (chat WS in v1). Defined as an
// interface so the registry package stays independent of the WS layer.
type InstallNotifier interface {
	NotifyInstall(targetAgentID string, sk StoredSkill) error
}

type RouteDeps struct {
	Store      *Store
	KnownTools []string
	Identity   IdentityProvider
	Installer  InstallNotifier
}

// RegisterRoutes wires registry HTTP handlers onto mux. Caller is responsible
// for the path prefix; routes are registered under /api/registry/.
func RegisterRoutes(mux *http.ServeMux, d *RouteDeps) {
	mux.HandleFunc("/api/registry/skills", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			d.handlePublish(w, r)
		case http.MethodGet:
			d.handleList(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/registry/skills/", func(w http.ResponseWriter, r *http.Request) {
		// path: /api/registry/skills/<name>            (GET / DELETE)
		// path: /api/registry/skills/<name>/history    (GET)
		// path: /api/registry/skills/<name>/<version>  (DELETE)
		d.handleSubroutes(w, r)
	})
	mux.HandleFunc("/api/registry/install_request", d.handleInstallRequest)
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

func (d *RouteDeps) handleList(w http.ResponseWriter, r *http.Request) {
	if _, err := d.Identity.ResolveActor(r); err != nil {
		writeJSON(w, 401, errBody{Code: "unauth"})
		return
	}
	q := r.URL.Query().Get("q")
	out, err := d.Store.List(q)
	if err != nil {
		writeJSON(w, 500, errBody{Code: "store", Reason: err.Error()})
		return
	}
	if out == nil {
		out = []Summary{}
	}
	writeJSON(w, 200, out)
}

func (d *RouteDeps) handleSubroutes(w http.ResponseWriter, r *http.Request) {
	actor, err := d.Identity.ResolveActor(r)
	if err != nil {
		writeJSON(w, 401, errBody{Code: "unauth"})
		return
	}
	rest := r.URL.Path[len("/api/registry/skills/"):]
	parts := splitPath(rest)
	switch len(parts) {
	case 1:
		// /api/registry/skills/<name>
		switch r.Method {
		case http.MethodGet:
			d.getOne(w, r, parts[0])
		default:
			http.Error(w, "method not allowed", 405)
		}
	case 2:
		switch parts[1] {
		case "history":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", 405)
				return
			}
			d.getHistory(w, r, parts[0])
		default:
			// /api/registry/skills/<name>/<version> — DELETE only
			if r.Method != http.MethodDelete {
				http.Error(w, "method not allowed", 405)
				return
			}
			d.deleteVersion(w, r, parts[0], parts[1], actor)
		}
	default:
		http.Error(w, "not found", 404)
	}
}

func splitPath(p string) []string {
	out := []string{}
	cur := ""
	for _, c := range p {
		if c == '/' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(c)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func (d *RouteDeps) getOne(w http.ResponseWriter, r *http.Request, name string) {
	versionStr := r.URL.Query().Get("version")
	version := 0
	if versionStr != "" {
		v, err := strconv.Atoi(versionStr)
		if err != nil {
			writeJSON(w, 400, errBody{Code: "bad_version", Reason: err.Error()})
			return
		}
		if v <= 0 {
			writeJSON(w, 400, errBody{Code: "bad_version", Reason: "version must be a positive integer"})
			return
		}
		version = v
	}
	got, err := d.Store.Get(name, version)
	if err == ErrNotFound {
		writeJSON(w, 404, errBody{Code: "not_found"})
		return
	}
	if err != nil {
		writeJSON(w, 500, errBody{Code: "store", Reason: err.Error()})
		return
	}
	writeJSON(w, 200, got)
}

func (d *RouteDeps) getHistory(w http.ResponseWriter, _ *http.Request, name string) {
	hist, err := d.Store.History(name)
	if err != nil {
		writeJSON(w, 500, errBody{Code: "store", Reason: err.Error()})
		return
	}
	if hist == nil {
		hist = []Summary{}
	}
	writeJSON(w, 200, hist)
}

func (d *RouteDeps) deleteVersion(w http.ResponseWriter, _ *http.Request, name, versionStr string, actor Actor) {
	if actor.Kind != "operator" || !actor.IsAdmin {
		writeJSON(w, 403, errBody{Code: "forbidden"})
		return
	}
	version, err := strconv.Atoi(versionStr)
	if err != nil {
		writeJSON(w, 400, errBody{Code: "bad_version", Reason: err.Error()})
		return
	}
	if version <= 0 {
		writeJSON(w, 400, errBody{Code: "bad_version", Reason: "version must be a positive integer"})
		return
	}
	if err := d.Store.SoftDelete(name, version, actor.ID); err == ErrNotFound {
		writeJSON(w, 404, errBody{Code: "not_found"})
		return
	} else if err != nil {
		writeJSON(w, 500, errBody{Code: "store", Reason: err.Error()})
		return
	}
	w.WriteHeader(204)
}

func (d *RouteDeps) handleInstallRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	actor, err := d.Identity.ResolveActor(r)
	if err != nil {
		writeJSON(w, 401, errBody{Code: "unauth"})
		return
	}
	if actor.Kind != "operator" {
		writeJSON(w, 403, errBody{Code: "forbidden", Reason: "operators only"})
		return
	}
	var req struct {
		Name          string `json:"name"`
		Version       int    `json:"version"` // 0 = latest
		TargetAgentID string `json:"target_agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody{Code: "bad_json", Reason: err.Error()})
		return
	}
	if req.Name == "" || req.TargetAgentID == "" {
		writeJSON(w, 400, errBody{Code: "missing_field"})
		return
	}
	got, err := d.Store.Get(req.Name, req.Version)
	if err == ErrNotFound {
		writeJSON(w, 404, errBody{Code: "not_found"})
		return
	}
	if err != nil {
		writeJSON(w, 500, errBody{Code: "store", Reason: err.Error()})
		return
	}
	if d.Installer == nil {
		writeJSON(w, 503, errBody{Code: "no_installer"})
		return
	}
	if err := d.Installer.NotifyInstall(req.TargetAgentID, *got); err != nil {
		writeJSON(w, 502, errBody{Code: "notify_failed", Reason: err.Error()})
		return
	}
	w.WriteHeader(202)
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
