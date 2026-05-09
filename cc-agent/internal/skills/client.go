package skills

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

// RegistryClient calls cc-control's /api/registry/* endpoints.
type RegistryClient struct {
	BaseURL    string
	AgentToken string
	ServerID   string // sent as X-Server-ID header for agent identity resolution
	HTTP       *http.Client
	TeamDir    string // where installed skills are written; empty = no file write
}

// StoredSkill mirrors cc-control's registry.StoredSkill — wire-compat decode target.
type StoredSkill struct {
	Skill
	Version        int    `json:"version"`
	AuthorServerID string `json:"author_server_id"`
	CreatedAtUnix  int64  `json:"created_at_unix"`
}

// Summary mirrors registry.Summary — list-row data, no full body.
type Summary struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	Version        int    `json:"version"`
	AuthorServerID string `json:"author_server_id"`
	CreatedAtUnix  int64  `json:"created_at_unix"`
}

func (c *RegistryClient) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c *RegistryClient) do(method, path string, body any) (*http.Response, error) {
	url := c.BaseURL + path
	var req *http.Request
	var err error
	if body != nil {
		raw, mErr := json.Marshal(body)
		if mErr != nil {
			return nil, fmt.Errorf("marshal: %w", mErr)
		}
		req, err = http.NewRequest(method, url, bytes.NewReader(raw))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
		}
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.AgentToken)
	if c.ServerID != "" {
		req.Header.Set("X-Server-ID", c.ServerID)
	}
	return c.httpClient().Do(req)
}

func (c *RegistryClient) Publish(s *Skill) (version int, err error) {
	resp, err := c.do("POST", "/api/registry/skills", s)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, decodeAPIError(resp)
	}
	var out struct {
		Version int `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, fmt.Errorf("decode: %w", err)
	}
	return out.Version, nil
}

// Install fetches a skill from the registry and (if TeamDir is set) writes it
// to <TeamDir>/<name>.json atomically. version=0 means latest.
func (c *RegistryClient) Install(name string, version int) (*StoredSkill, error) {
	q := ""
	if version > 0 {
		q = "?" + url.Values{"version": []string{fmt.Sprint(version)}}.Encode()
	}
	resp, err := c.do("GET", "/api/registry/skills/"+name+q, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, decodeAPIError(resp)
	}
	var got StoredSkill
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if c.TeamDir == "" {
		return &got, nil
	}
	if err := os.MkdirAll(c.TeamDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir team: %w", err)
	}
	final := filepath.Join(c.TeamDir, got.Name+".json")
	tmp := final + ".tmp"
	body, _ := json.MarshalIndent(got.Skill, "", "  ")
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return nil, fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return nil, fmt.Errorf("rename: %w", err)
	}
	return &got, nil
}

func (c *RegistryClient) List(query string) ([]Summary, error) {
	path := "/api/registry/skills"
	if query != "" {
		path += "?" + url.Values{"q": []string{query}}.Encode()
	}
	resp, err := c.do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, decodeAPIError(resp)
	}
	var out []Summary
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *RegistryClient) History(name string) ([]Summary, error) {
	resp, err := c.do("GET", "/api/registry/skills/"+name+"/history", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, decodeAPIError(resp)
	}
	var out []Summary
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func decodeAPIError(resp *http.Response) error {
	var e struct {
		Code   string `json:"code"`
		Field  string `json:"field"`
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&e)
	return fmt.Errorf("registry error: %s (%s) %s", e.Code, e.Field, e.Reason)
}
