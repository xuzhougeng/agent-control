package registry

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func newTestServer(t *testing.T, actor Actor) (*httptest.Server, *Store) {
	t.Helper()
	dir := t.TempDir()
	st, err := OpenStore(filepath.Join(dir, "registry.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	mux := http.NewServeMux()
	RegisterRoutes(mux, &RouteDeps{
		Store:      st,
		KnownTools: []string{"bash", "read", "write", "grep", "glob", "sysinfo", "proclist", "logtail"},
		Identity:   stubIdentity{actor: actor},
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, st
}

type stubIdentity struct{ actor Actor }

func (s stubIdentity) ResolveActor(*http.Request) (Actor, error) { return s.actor, nil }

func TestPublishHandler_Success(t *testing.T) {
	srv, _ := newTestServer(t, Actor{Kind: "agent", ID: "ops-01"})
	body, _ := json.Marshal(Skill{Name: "nginx-triage", Prompt: "p", Tools: []string{"bash"}})
	resp, err := http.Post(srv.URL+"/api/registry/skills", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		b, _ := json.Marshal(resp.Status)
		t.Fatalf("status %d: %s", resp.StatusCode, b)
	}
	var got struct{ Version int }
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got.Version != 1 {
		t.Fatalf("version=%d, want 1", got.Version)
	}
}

func TestPublishHandler_ValidationError(t *testing.T) {
	srv, _ := newTestServer(t, Actor{Kind: "agent", ID: "ops-01"})
	body, _ := json.Marshal(Skill{Name: "BAD UPPER", Prompt: "p"})
	resp, _ := http.Post(srv.URL+"/api/registry/skills", "application/json", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status=%d, want 400", resp.StatusCode)
	}
	var e struct{ Code, Field string }
	_ = json.NewDecoder(resp.Body).Decode(&e)
	if e.Code != "invalid_skill" || e.Field != "name" {
		t.Fatalf("body=%+v", e)
	}
}

func TestPublishHandler_RequiresAuth(t *testing.T) {
	dir := t.TempDir()
	st, _ := OpenStore(filepath.Join(dir, "r.db"))
	defer st.Close()
	mux := http.NewServeMux()
	RegisterRoutes(mux, &RouteDeps{
		Store:      st,
		KnownTools: []string{"bash"},
		Identity:   stubIdentityErr{},
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	body := strings.NewReader(`{"name":"x","prompt":"p"}`)
	resp, _ := http.Post(srv.URL+"/api/registry/skills", "application/json", body)
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status=%d, want 401", resp.StatusCode)
	}
}

type stubIdentityErr struct{}

func (stubIdentityErr) ResolveActor(*http.Request) (Actor, error) {
	return Actor{}, errAuth("nope")
}

type errAuth string

func (e errAuth) Error() string { return string(e) }
