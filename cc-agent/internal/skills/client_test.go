package skills

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClient_Publish_RoundTrip(t *testing.T) {
	got := struct {
		path     string
		method   string
		auth     string
		serverID string
		body     Skill
	}{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path = r.URL.Path
		got.method = r.Method
		got.auth = r.Header.Get("Authorization")
		got.serverID = r.Header.Get("X-Server-ID")
		_ = json.NewDecoder(r.Body).Decode(&got.body)
		_, _ = io.WriteString(w, `{"name":"x","version":7}`)
	}))
	defer srv.Close()
	c := &RegistryClient{BaseURL: srv.URL, AgentToken: "tok", ServerID: "ops-01", HTTP: srv.Client()}
	v, err := c.Publish(&Skill{Name: "x", Prompt: "p"})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if v != 7 {
		t.Fatalf("got %d, want 7", v)
	}
	if got.path != "/api/registry/skills" || got.method != "POST" || !strings.HasSuffix(got.auth, " tok") || got.serverID != "ops-01" {
		t.Fatalf("request: %+v", got)
	}
	if got.body.Name != "x" {
		t.Fatalf("body: %+v", got.body)
	}
}

func TestClient_Publish_PropagatesValidationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(400)
		_, _ = io.WriteString(w, `{"code":"invalid_skill","field":"name","reason":"bad"}`)
	}))
	defer srv.Close()
	c := &RegistryClient{BaseURL: srv.URL, AgentToken: "tok", ServerID: "ops-01", HTTP: srv.Client()}
	_, err := c.Publish(&Skill{Name: "BAD", Prompt: "p"})
	if err == nil {
		t.Fatalf("want error")
	}
	if !strings.Contains(err.Error(), "invalid_skill") {
		t.Fatalf("err = %v", err)
	}
}

func TestClient_Install_AtomicWriteAndReload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"name":"nginx-triage","version":3,"prompt":"p","tools":["bash"],"author_server_id":"ops-01","created_at_unix":100}`)
	}))
	defer srv.Close()
	dir := t.TempDir()
	teamDir := filepath.Join(dir, "team")
	c := &RegistryClient{BaseURL: srv.URL, AgentToken: "t", ServerID: "ops-A", HTTP: srv.Client(), TeamDir: teamDir}
	got, err := c.Install("nginx-triage", 0)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got.Version != 3 {
		t.Fatalf("version=%d", got.Version)
	}
	written, err := os.ReadFile(filepath.Join(teamDir, "nginx-triage.json"))
	if err != nil {
		t.Fatalf("read installed: %v", err)
	}
	if !strings.Contains(string(written), `"nginx-triage"`) {
		t.Fatalf("body: %s", written)
	}
	// Repeated install must succeed (idempotent).
	if _, err := c.Install("nginx-triage", 0); err != nil {
		t.Fatalf("second Install: %v", err)
	}
}

func TestClient_Install_VersionedURL(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		_, _ = io.WriteString(w, `{"name":"x","version":2,"prompt":"p","author_server_id":"o","created_at_unix":1}`)
	}))
	defer srv.Close()
	dir := t.TempDir()
	c := &RegistryClient{BaseURL: srv.URL, AgentToken: "t", ServerID: "ops-A", HTTP: srv.Client(), TeamDir: dir}
	if _, err := c.Install("x", 2); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotURL, "version=2") {
		t.Fatalf("url=%s", gotURL)
	}
}

func TestClient_List(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[{"name":"x","description":"demo","version":3,"author_server_id":"a","created_at_unix":1}]`)
	}))
	defer srv.Close()
	c := &RegistryClient{BaseURL: srv.URL, AgentToken: "t", ServerID: "ops-A", HTTP: srv.Client()}
	got, err := c.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "x" {
		t.Fatalf("got %+v", got)
	}
}

func TestClient_List_PassesQuery(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		_, _ = io.WriteString(w, `[]`)
	}))
	defer srv.Close()
	c := &RegistryClient{BaseURL: srv.URL, AgentToken: "t", ServerID: "ops-A", HTTP: srv.Client()}
	_, _ = c.List("nginx")
	if !strings.Contains(gotURL, "q=nginx") {
		t.Fatalf("url=%s", gotURL)
	}
}

func TestClient_History(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/history") {
			t.Errorf("path=%s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `[{"name":"x","version":1,"author_server_id":"a","created_at_unix":1}]`)
	}))
	defer srv.Close()
	c := &RegistryClient{BaseURL: srv.URL, AgentToken: "t", ServerID: "ops-A", HTTP: srv.Client()}
	hist, err := c.History("x")
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 1 {
		t.Fatalf("hist=%+v", hist)
	}
}
