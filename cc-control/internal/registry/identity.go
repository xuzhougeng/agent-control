package registry

import (
	"errors"
	"net/http"
	"strings"

	"cc-control/internal/auth"
)

// Actor represents the authenticated principal making a registry request.
//
// Kind is either "agent" (for agent tokens) or "operator" (for UI/tenant/admin
// tokens). ID is the stable identifier for that principal: server_id for
// agents, tenant_id for UI/tenant operators, and token_id for admin tokens.
// IsAdmin is true for tenant owners (UI tokens with RoleOwner) and global
// admin tokens.
type Actor struct {
	Kind    string
	ID      string
	IsAdmin bool
}

// AuthBackend is the subset of the auth store used by the identity resolver.
// The real *auth.Store satisfies this interface; tests inject a fake.
type AuthBackend interface {
	Lookup(token string) (*auth.TokenRecord, bool)
}

// IdentityResolver bridges cc-control's auth tokens to a registry-level Actor.
type IdentityResolver struct {
	Auth AuthBackend
}

// ResolveActor extracts the bearer token from req, looks it up in the auth
// backend, and maps the resulting TokenRecord (plus the X-Server-ID header
// for agent tokens) to an Actor.
func (r *IdentityResolver) ResolveActor(req *http.Request) (Actor, error) {
	tok := bearer(req)
	if tok == "" {
		return Actor{}, errors.New("missing bearer token")
	}
	rec, ok := r.Auth.Lookup(tok)
	if !ok || rec == nil {
		return Actor{}, errors.New("unknown token")
	}
	if rec.Revoked {
		return Actor{}, errors.New("revoked token")
	}
	switch rec.Type {
	case auth.TokenTypeAgent:
		id := strings.TrimSpace(req.Header.Get("X-Server-ID"))
		if id == "" {
			id = rec.Name
		}
		if id == "" {
			return Actor{}, errors.New("agent token missing server_id (header or token name)")
		}
		return Actor{Kind: "agent", ID: id}, nil
	case auth.TokenTypeUI:
		return Actor{Kind: "operator", ID: rec.TenantID, IsAdmin: rec.Role == auth.RoleOwner}, nil
	case auth.TokenTypeTenant:
		return Actor{Kind: "operator", ID: rec.TenantID}, nil
	case auth.TokenTypeAdmin:
		return Actor{Kind: "operator", ID: rec.TokenID, IsAdmin: true}, nil
	default:
		return Actor{}, errors.New("unsupported token type")
	}
}

// bearer extracts a bearer token from either the Authorization header
// ("Bearer <token>") or the ?token= query parameter.
func bearer(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(h) >= 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}
