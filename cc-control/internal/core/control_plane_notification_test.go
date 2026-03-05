package core

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func newNotificationTestControlPlane(t *testing.T) *ControlPlane {
	t.Helper()
	auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
	cp, err := NewControlPlane(Config{AuditPath: auditPath})
	if err != nil {
		t.Fatalf("new control plane: %v", err)
	}
	t.Cleanup(func() { _ = cp.Close() })
	return cp
}

func TestPublishNotification_BroadcastsWithinTenantAndStoresHistory(t *testing.T) {
	cp := newNotificationTestControlPlane(t)

	subT1 := &Subscriber{ID: "sub-t1", TenantID: "t1", Send: make(chan Envelope, 1)}
	subT2 := &Subscriber{ID: "sub-t2", TenantID: "t2", Send: make(chan Envelope, 1)}
	cp.RegisterSubscriber(subT1)
	cp.RegisterSubscriber(subT2)
	t.Cleanup(func() {
		cp.UnregisterSubscriber(subT1)
		cp.UnregisterSubscriber(subT2)
	})

	ev, err := cp.PublishNotification("cli:test", "t1", PublishNotificationRequest{
		Title:   "Build Done",
		Message: "Build and tests finished successfully.",
		Level:   NotificationLevelSuccess,
		Source:  "ci",
	})
	if err != nil {
		t.Fatalf("publish notification: %v", err)
	}
	if ev.Kind != "notification" {
		t.Fatalf("expected kind=notification, got %q", ev.Kind)
	}
	if ev.Level != NotificationLevelSuccess {
		t.Fatalf("expected level=success, got %q", ev.Level)
	}

	select {
	case msg := <-subT1.Send:
		if msg.Type != "event" {
			t.Fatalf("expected ws event, got %q", msg.Type)
		}
		var got NotificationEvent
		if err := json.Unmarshal(msg.Data, &got); err != nil {
			t.Fatalf("decode event payload: %v", err)
		}
		if got.NotificationID != ev.NotificationID {
			t.Fatalf("expected notification_id %q, got %q", ev.NotificationID, got.NotificationID)
		}
	case <-time.After(time.Second):
		t.Fatal("expected notification for tenant t1 subscriber")
	}

	select {
	case msg := <-subT2.Send:
		t.Fatalf("did not expect tenant t2 message, got type=%q", msg.Type)
	default:
	}

	recent := cp.GetRecentNotificationEvents("t1", 10)
	if len(recent) != 1 {
		t.Fatalf("expected 1 stored notification, got %d", len(recent))
	}
	if recent[0].NotificationID != ev.NotificationID {
		t.Fatalf("expected stored notification_id %q, got %q", ev.NotificationID, recent[0].NotificationID)
	}
}

func TestPublishNotification_WithSessionIDBackfillsServerID(t *testing.T) {
	cp := newNotificationTestControlPlane(t)
	cp.mu.Lock()
	cp.servers["srv-1"] = &Server{TenantID: "t1", ServerID: "srv-1", Status: ServerOnline}
	cp.sessions["sess-1"] = &Session{
		TenantID:  "t1",
		SessionID: "sess-1",
		ServerID:  "srv-1",
		Status:    SessionRunning,
	}
	cp.mu.Unlock()

	ev, err := cp.PublishNotification("cli:test", "t1", PublishNotificationRequest{
		SessionID: "sess-1",
		Message:   "Session task completed.",
	})
	if err != nil {
		t.Fatalf("publish notification: %v", err)
	}
	if ev.ServerID != "srv-1" {
		t.Fatalf("expected server_id srv-1, got %q", ev.ServerID)
	}
}
