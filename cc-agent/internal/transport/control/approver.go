package control

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// RemoteApprover blocks destructive bash commands until a human in the
// cc-control UI approves or rejects them. Implements tools.Approver.
//
// It is bound to a Client. The client's inbound message loop routes
// approval_decision envelopes back into the matching pending request.
type RemoteApprover struct {
	client  *Client
	timeout time.Duration

	mu       sync.Mutex
	pending  map[string]chan ApprovalDecisionPayload
}

// NewRemoteApprover constructs an approver with the default 5 minute
// timeout. Use NewRemoteApproverWithTimeout to pick a different deadline
// (e.g. 30s for real-time ops, 30min for overnight runs).
func NewRemoteApprover(c *Client) *RemoteApprover {
	return NewRemoteApproverWithTimeout(c, 5*time.Minute)
}

// NewRemoteApproverWithTimeout constructs an approver with a caller-chosen
// per-request deadline. Values <= 0 fall back to 5 minutes.
func NewRemoteApproverWithTimeout(c *Client, timeout time.Duration) *RemoteApprover {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &RemoteApprover{
		client:  c,
		timeout: timeout,
		pending: map[string]chan ApprovalDecisionPayload{},
	}
}

// Approve fires an approval_request to the control plane and blocks until a
// matching decision arrives, ctx cancels, or the per-request timeout elapses.
// Timeout / cancel both report as denied so the model sees a clear rejection
// and can adapt.
func (a *RemoteApprover) Approve(ctx context.Context, command, reason string) (bool, error) {
	if a.client == nil {
		return false, errors.New("remote approver: no client")
	}
	requestID, err := newRequestID()
	if err != nil {
		return false, err
	}
	ch := make(chan ApprovalDecisionPayload, 1)
	a.mu.Lock()
	a.pending[requestID] = ch
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.pending, requestID)
		a.mu.Unlock()
	}()

	// We do not yet know which session this is for; the agent.Run goroutine
	// has the session_id in its closure. The Client tracks the current
	// in-flight session via context value.
	sessionID, instanceID := SessionFromContext(ctx)
	if sessionID == "" {
		return false, errors.New("remote approver: no session in context (only chat-mode bridge supports remote approvals)")
	}

	payload, _ := json.Marshal(ApprovalRequestPayload{
		RequestID: requestID,
		Command:   command,
		Reason:    reason,
		TtlMS:     a.timeout.Milliseconds(),
	})
	env := newEnvelope("approval_request", a.client.opts.ServerID, sessionID)
	env.InstanceID = instanceID
	env.Data = payload
	if err := a.client.enqueue(env); err != nil {
		return false, fmt.Errorf("send approval_request: %w", err)
	}

	select {
	case d := <-ch:
		return d.Approved, nil
	case <-time.After(a.timeout):
		return false, nil
	case <-ctx.Done():
		return false, nil
	}
}

// dispatch is called by the Client's read loop when an approval_decision
// envelope arrives.
func (a *RemoteApprover) dispatch(d ApprovalDecisionPayload) {
	a.mu.Lock()
	ch := a.pending[d.RequestID]
	a.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- d:
	default:
	}
}

func newRequestID() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "ar_" + hex.EncodeToString(b[:]), nil
}

// sessionContextKey identifies the chat-mode session+instance currently
// driving an agent.Run. The control client stamps this on the context it
// hands to RunWithListener so RemoteApprover can find the right session.
type sessionContextKey struct{}

type sessionContext struct {
	SessionID  string
	InstanceID string
}

// WithSession threads chat session + instance ids through the agent loop so
// RemoteApprover can address its approval_request.
func WithSession(ctx context.Context, sessionID, instanceID string) context.Context {
	return context.WithValue(ctx, sessionContextKey{}, sessionContext{SessionID: sessionID, InstanceID: instanceID})
}

// SessionFromContext recovers the session ids stamped by WithSession.
func SessionFromContext(ctx context.Context) (sessionID, instanceID string) {
	v, _ := ctx.Value(sessionContextKey{}).(sessionContext)
	return v.SessionID, v.InstanceID
}
