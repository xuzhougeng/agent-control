package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"cc-control/internal/auth"
	"cc-control/internal/core"
)

func newNotificationTestServer(t *testing.T) (*Server, *auth.Store) {
	t.Helper()
	cp, err := core.NewControlPlane(core.Config{
		AuditPath: filepath.Join(t.TempDir(), "audit.jsonl"),
	})
	if err != nil {
		t.Fatalf("new control plane: %v", err)
	}
	t.Cleanup(func() { _ = cp.Close() })

	tokens := auth.NewStore()
	t.Cleanup(func() { _ = tokens.Close() })

	return &Server{
		CP:     cp,
		Tokens: tokens,
		UIDir:  "../../cc-web",
	}, tokens
}

func TestHandleNotifications_UIFlow(t *testing.T) {
	srv, tokens := newNotificationTestServer(t)
	if _, err := tokens.SeedToken("ui-token", auth.TokenTypeUI, auth.RoleOperator, "t1", "ui"); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	router := srv.Router()

	postBody := map[string]any{
		"title":   "Build Done",
		"message": "All tests passed.",
		"level":   "success",
		"source":  "ci",
	}
	raw, _ := json.Marshal(postBody)
	req := httptest.NewRequest(http.MethodPost, "/api/notifications", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer ui-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	var created struct {
		OK           bool                   `json:"ok"`
		Notification core.NotificationEvent `json:"notification"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if !created.OK || created.Notification.Kind != "notification" {
		t.Fatalf("unexpected create payload: %+v", created)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/notifications", nil)
	getReq.Header.Set("Authorization", "Bearer ui-token")
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}

	var listed struct {
		Notifications []core.NotificationEvent `json:"notifications"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listed.Notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(listed.Notifications))
	}
}

func TestHandleTenantNotifications_TenantTokenFlow(t *testing.T) {
	srv, tokens := newNotificationTestServer(t)
	if _, err := tokens.SeedToken("tenant-token", auth.TokenTypeTenant, "", "tenant-a", "tenant"); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	router := srv.Router()

	postBody := map[string]any{
		"message": "Background task finished.",
	}
	raw, _ := json.Marshal(postBody)
	req := httptest.NewRequest(http.MethodPost, "/tenant/notifications", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer tenant-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/tenant/notifications?limit=5", nil)
	getReq.Header.Set("Authorization", "Bearer tenant-token")
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}

	var listed struct {
		Notifications []core.NotificationEvent `json:"notifications"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listed.Notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(listed.Notifications))
	}
	if listed.Notifications[0].TenantID != "tenant-a" {
		t.Fatalf("expected tenant_id tenant-a, got %q", listed.Notifications[0].TenantID)
	}
}
