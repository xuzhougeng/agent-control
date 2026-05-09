package skills

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
