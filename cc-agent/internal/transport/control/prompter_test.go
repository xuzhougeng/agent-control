package control

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"cc-agent/internal/secrets"
)

func TestRemotePrompter_GrantReturnsSecret(t *testing.T) {
	c := &Client{opts: Options{ServerID: "srv"}, instances: map[string]*chatInstance{}, send: make(chan Envelope, 16)}
	p := NewRemotePrompter(c)
	p.timeout = 2 * time.Second
	ctx := WithSession(context.Background(), "s-1", "i-1")

	done := make(chan struct {
		secret []byte
		ok     bool
		err    error
	}, 1)
	go func() {
		secret, ok, err := p.Prompt(ctx, secrets.Request{Name: "sudo", Reason: "auth", Command: "sudo id"})
		done <- struct {
			secret []byte
			ok     bool
			err    error
		}{secret, ok, err}
	}()

	select {
	case env := <-c.send:
		if env.Type != "credential_request" {
			t.Fatalf("type = %q", env.Type)
		}
		var req CredentialRequestPayload
		if err := json.Unmarshal(env.Data, &req); err != nil {
			t.Fatal(err)
		}
		if req.Name != "sudo" || req.RequestID == "" {
			t.Fatalf("payload = %+v", req)
		}
		if strings.Contains(string(env.Data), "s3cret") {
			t.Fatal("request must not contain a secret")
		}
		p.dispatch(CredentialDecisionPayload{RequestID: req.RequestID, Granted: true, Secret: "s3cret"})
	case <-time.After(time.Second):
		t.Fatal("credential_request not enqueued")
	}

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatal(r.err)
		}
		if !r.ok || string(r.secret) != "s3cret" {
			t.Fatalf("got ok=%v secret=%q", r.ok, r.secret)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Prompt never returned")
	}
}

func TestRemotePrompter_Reject(t *testing.T) {
	c := &Client{opts: Options{ServerID: "srv"}, instances: map[string]*chatInstance{}, send: make(chan Envelope, 16)}
	p := NewRemotePrompter(c)
	p.timeout = 2 * time.Second
	ctx := WithSession(context.Background(), "s-1", "i-1")
	done := make(chan bool, 1)
	go func() {
		_, ok, _ := p.Prompt(ctx, secrets.Request{Name: "mysql/prod"})
		done <- ok
	}()
	select {
	case env := <-c.send:
		var req CredentialRequestPayload
		_ = json.Unmarshal(env.Data, &req)
		p.dispatch(CredentialDecisionPayload{RequestID: req.RequestID, Granted: false})
	case <-time.After(time.Second):
		t.Fatal("credential_request not enqueued")
	}
	select {
	case ok := <-done:
		if ok {
			t.Fatal("expected deny")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Prompt never returned")
	}
}
