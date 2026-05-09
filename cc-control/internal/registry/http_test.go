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
	resp, err := http.Post(srv.URL+"/api/registry/skills", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
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
	resp, err := http.Post(srv.URL+"/api/registry/skills", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
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

func TestList_Empty(t *testing.T) {
	srv, _ := newTestServer(t, Actor{Kind: "operator", ID: "alice"})
	resp, err := http.Get(srv.URL + "/api/registry/skills")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var got []Summary
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if len(got) != 0 {
		t.Fatalf("got %d, want 0", len(got))
	}
}

func TestList_AfterPublish(t *testing.T) {
	srv, st := newTestServer(t, Actor{Kind: "agent", ID: "ops-01"})
	_, _ = st.Publish(&Skill{Name: "x", Prompt: "p", Description: "demo"}, "ops-01")
	resp, err := http.Get(srv.URL + "/api/registry/skills")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got []Summary
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if len(got) != 1 || got[0].Name != "x" {
		t.Fatalf("got %+v", got)
	}
}

func TestGetOne_Latest(t *testing.T) {
	srv, st := newTestServer(t, Actor{Kind: "agent", ID: "ops-01"})
	_, _ = st.Publish(&Skill{Name: "x", Prompt: "p1"}, "ops-01")
	_, _ = st.Publish(&Skill{Name: "x", Prompt: "p2"}, "ops-01")
	resp, err := http.Get(srv.URL + "/api/registry/skills/x")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var got StoredSkill
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got.Version != 2 || got.Prompt != "p2" {
		t.Fatalf("got %+v", got)
	}
}

func TestGetOne_PinnedVersion(t *testing.T) {
	srv, st := newTestServer(t, Actor{Kind: "agent", ID: "ops-01"})
	_, _ = st.Publish(&Skill{Name: "x", Prompt: "p1"}, "ops-01")
	_, _ = st.Publish(&Skill{Name: "x", Prompt: "p2"}, "ops-01")
	resp, err := http.Get(srv.URL + "/api/registry/skills/x?version=1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got StoredSkill
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got.Prompt != "p1" {
		t.Fatalf("got %+v", got)
	}
}

func TestGetOne_NotFound(t *testing.T) {
	srv, _ := newTestServer(t, Actor{Kind: "agent", ID: "ops-01"})
	resp, err := http.Get(srv.URL + "/api/registry/skills/missing")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("status=%d, want 404", resp.StatusCode)
	}
}

func TestHistory(t *testing.T) {
	srv, st := newTestServer(t, Actor{Kind: "agent", ID: "ops-01"})
	_, _ = st.Publish(&Skill{Name: "x", Prompt: "p1"}, "ops-01")
	_, _ = st.Publish(&Skill{Name: "x", Prompt: "p2"}, "ops-01")
	resp, err := http.Get(srv.URL + "/api/registry/skills/x/history")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var hist []Summary
	_ = json.NewDecoder(resp.Body).Decode(&hist)
	if len(hist) != 2 {
		t.Fatalf("got %d, want 2", len(hist))
	}
}

func TestDelete_AdminCanSoftDelete(t *testing.T) {
	srv, st := newTestServer(t, Actor{Kind: "operator", ID: "admin", IsAdmin: true})
	_, _ = st.Publish(&Skill{Name: "x", Prompt: "p"}, "ops-01")
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/registry/skills/x/1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("status=%d, want 204", resp.StatusCode)
	}
	if _, err := st.Latest("x"); err != ErrNotFound {
		t.Fatalf("Latest after delete: %v", err)
	}
}

func TestDelete_NonAdminForbidden(t *testing.T) {
	srv, st := newTestServer(t, Actor{Kind: "operator", ID: "alice", IsAdmin: false})
	_, _ = st.Publish(&Skill{Name: "x", Prompt: "p"}, "ops-01")
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/registry/skills/x/1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("status=%d, want 403", resp.StatusCode)
	}
}

func TestDelete_AgentForbidden(t *testing.T) {
	srv, st := newTestServer(t, Actor{Kind: "agent", ID: "ops-01"})
	_, _ = st.Publish(&Skill{Name: "x", Prompt: "p"}, "ops-01")
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/registry/skills/x/1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("status=%d, want 403 (agents can't delete)", resp.StatusCode)
	}
}
