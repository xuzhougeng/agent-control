package registry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
)

// stubAgentIdentity resolves to an agent actor identified by the X-Server-ID
// header — bypasses auth.Store for this end-to-end smoke.
type stubAgentIdentity struct{}

func (stubAgentIdentity) ResolveActor(r *http.Request) (Actor, error) {
	id := r.Header.Get("X-Server-ID")
	if id == "" {
		id = "anon-agent"
	}
	return Actor{Kind: "agent", ID: id}, nil
}

func TestRegistry_PublishInstallRollback_E2E(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "registry.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	deps := &RouteDeps{
		Store:      store,
		KnownTools: []string{"bash", "read", "write", "grep", "glob", "sysinfo", "proclist", "logtail"},
		Identity:   stubAgentIdentity{},
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, deps)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	publish := func(serverID string, sk Skill) (int, error) {
		body, _ := json.Marshal(sk)
		req, _ := http.NewRequest("POST", srv.URL+"/api/registry/skills", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Server-ID", serverID)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			b, _ := io.ReadAll(resp.Body)
			return 0, fmt.Errorf("publish status=%d body=%s", resp.StatusCode, b)
		}
		var out struct {
			Version int `json:"version"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out.Version, nil
	}

	getOne := func(serverID, name string, version int) (StoredSkill, error) {
		path := "/api/registry/skills/" + name
		if version > 0 {
			path += "?" + url.Values{"version": []string{fmt.Sprint(version)}}.Encode()
		}
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		req.Header.Set("X-Server-ID", serverID)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return StoredSkill{}, err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			b, _ := io.ReadAll(resp.Body)
			return StoredSkill{}, fmt.Errorf("get status=%d body=%s", resp.StatusCode, b)
		}
		var out StoredSkill
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out, nil
	}

	listAll := func(serverID string) ([]Summary, error) {
		req, _ := http.NewRequest("GET", srv.URL+"/api/registry/skills", nil)
		req.Header.Set("X-Server-ID", serverID)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		var out []Summary
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out, nil
	}

	historyOf := func(serverID, name string) ([]Summary, error) {
		req, _ := http.NewRequest("GET", srv.URL+"/api/registry/skills/"+name+"/history", nil)
		req.Header.Set("X-Server-ID", serverID)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		var out []Summary
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out, nil
	}

	const skillName = "demo-skill"

	// 1. A publishes v1.
	v, err := publish("ops-A", Skill{Name: skillName, Prompt: "p1", Description: "demo", Tools: []string{"bash"}})
	if err != nil || v != 1 {
		t.Fatalf("A.publish v1 = %d, %v (want 1)", v, err)
	}

	// 2. B lists, sees the skill.
	rows, err := listAll("ops-B")
	if err != nil {
		t.Fatalf("B.list: %v", err)
	}
	if len(rows) != 1 || rows[0].Name != skillName {
		t.Fatalf("B.list = %+v, want one row [%s]", rows, skillName)
	}

	// 3. B installs latest (gets v1).
	got, err := getOne("ops-B", skillName, 0)
	if err != nil {
		t.Fatalf("B.get latest: %v", err)
	}
	if got.Version != 1 || got.Prompt != "p1" {
		t.Fatalf("B.get latest = %+v, want v1/p1", got)
	}

	// 4. A publishes v2.
	v, err = publish("ops-A", Skill{Name: skillName, Prompt: "p2", Tools: []string{"bash"}})
	if err != nil || v != 2 {
		t.Fatalf("A.publish v2 = %d, %v (want 2)", v, err)
	}

	// 5. B installs latest, gets v2.
	got, err = getOne("ops-B", skillName, 0)
	if err != nil {
		t.Fatalf("B.get latest after v2: %v", err)
	}
	if got.Version != 2 || got.Prompt != "p2" {
		t.Fatalf("B.get latest after v2 = %+v, want v2/p2", got)
	}

	// 6. B rolls back to v1 (explicit version).
	got, err = getOne("ops-B", skillName, 1)
	if err != nil {
		t.Fatalf("B.get @1: %v", err)
	}
	if got.Version != 1 || got.Prompt != "p1" {
		t.Fatalf("B.get @1 = %+v, want v1/p1", got)
	}

	// 7. History shows both versions in order.
	hist, err := historyOf("ops-A", skillName)
	if err != nil {
		t.Fatalf("A.history: %v", err)
	}
	if len(hist) != 2 || hist[0].Version != 1 || hist[1].Version != 2 {
		t.Fatalf("A.history = %+v, want [v1, v2]", hist)
	}
}
