package skills

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// RegistryClient calls cc-control's /api/registry/* endpoints.
type RegistryClient struct {
	BaseURL    string
	AgentToken string
	ServerID   string // sent as X-Server-ID header for agent identity resolution
	HTTP       *http.Client
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

func decodeAPIError(resp *http.Response) error {
	var e struct {
		Code   string `json:"code"`
		Field  string `json:"field"`
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&e)
	return fmt.Errorf("registry error: %s (%s) %s", e.Code, e.Field, e.Reason)
}
