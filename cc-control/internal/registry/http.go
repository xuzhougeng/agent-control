package registry

import (
	"encoding/json"
	"fmt"
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
		var err error
		version, err = parseInt(versionStr)
		if err != nil {
			writeJSON(w, 400, errBody{Code: "bad_version", Reason: err.Error()})
			return
		}
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

func parseInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number: %q", s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

func (d *RouteDeps) deleteVersion(w http.ResponseWriter, _ *http.Request, _ string, _ string, _ Actor) {
	http.Error(w, "delete not implemented", http.StatusNotImplemented)
}

func (d *RouteDeps) handleInstallRequest(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "install_request not implemented", http.StatusNotImplemented)
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
