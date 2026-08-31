package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"cc-agent/internal/secrets"
)

// RemotePrompter asks the operator for a named secret via cc-control.
// Implements secrets.Prompter.
type RemotePrompter struct {
	client  *Client
	timeout time.Duration

	mu      sync.Mutex
	pending map[string]chan CredentialDecisionPayload
}

func NewRemotePrompter(c *Client) *RemotePrompter {
	return NewRemotePrompterWithTimeout(c, 5*time.Minute)
}

func NewRemotePrompterWithTimeout(c *Client, timeout time.Duration) *RemotePrompter {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &RemotePrompter{
		client:  c,
		timeout: timeout,
		pending: map[string]chan CredentialDecisionPayload{},
	}
}

func (p *RemotePrompter) Prompt(ctx context.Context, req secrets.Request) ([]byte, bool, error) {
	if p.client == nil {
		return nil, false, errors.New("remote prompter: no client")
	}
	requestID, err := newRequestID()
	if err != nil {
		return nil, false, err
	}
	ch := make(chan CredentialDecisionPayload, 1)
	p.mu.Lock()
	p.pending[requestID] = ch
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.pending, requestID)
		p.mu.Unlock()
	}()

	sessionID, instanceID := SessionFromContext(ctx)
	if sessionID == "" {
		return nil, false, errors.New("remote prompter: no session in context")
	}

	payload, _ := json.Marshal(CredentialRequestPayload{
		RequestID: requestID,
		Name:      req.Name,
		Kind:      "password",
		Reason:    req.Reason,
		Command:   req.Command,
		TtlMS:     p.timeout.Milliseconds(),
	})
	env := newEnvelope("credential_request", p.client.opts.ServerID, sessionID)
	env.InstanceID = instanceID
	env.Data = payload
	if err := p.client.enqueue(env); err != nil {
		return nil, false, fmt.Errorf("send credential_request: %w", err)
	}

	select {
	case d := <-ch:
		if !d.Granted || d.Secret == "" {
			return nil, false, nil
		}
		out := []byte(d.Secret)
		d.Secret = ""
		return out, true, nil
	case <-time.After(p.timeout):
		return nil, false, nil
	case <-ctx.Done():
		return nil, false, nil
	}
}

func (p *RemotePrompter) dispatch(d CredentialDecisionPayload) {
	p.mu.Lock()
	ch := p.pending[d.RequestID]
	p.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- d:
	default:
	}
}
