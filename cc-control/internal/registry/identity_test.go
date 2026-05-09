package registry

import (
	"net/http/httptest"
	"testing"

	"cc-control/internal/auth"
)

type fakeAuth struct {
	rec *auth.TokenRecord
	ok  bool
}

func (f *fakeAuth) Lookup(token string) (*auth.TokenRecord, bool) {
	return f.rec, f.ok
}

func TestResolveActor_AgentTokenWithServerIDHeader(t *testing.T) {
	res := &IdentityResolver{
		Auth: &fakeAuth{rec: &auth.TokenRecord{Type: auth.TokenTypeAgent, TenantID: "t1"}, ok: true},
	}
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Authorization", "Bearer abc")
	r.Header.Set("X-Server-ID", "ops-01")
	a, err := res.ResolveActor(r)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if a.Kind != "agent" || a.ID != "ops-01" || a.IsAdmin {
		t.Fatalf("got %+v, want agent/ops-01/!admin", a)
	}
}

func TestResolveActor_AgentTokenFallbackToTokenName(t *testing.T) {
	res := &IdentityResolver{
		Auth: &fakeAuth{rec: &auth.TokenRecord{Type: auth.TokenTypeAgent, Name: "ops-fallback"}, ok: true},
	}
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Authorization", "Bearer abc")
	a, err := res.ResolveActor(r)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if a.ID != "ops-fallback" {
		t.Fatalf("got %+v, want id=ops-fallback", a)
	}
}

func TestResolveActor_AgentTokenNoIdentifier_Errors(t *testing.T) {
	res := &IdentityResolver{
		Auth: &fakeAuth{rec: &auth.TokenRecord{Type: auth.TokenTypeAgent}, ok: true},
	}
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Authorization", "Bearer abc")
	if _, err := res.ResolveActor(r); err == nil {
		t.Fatalf("want error, got nil")
	}
}

func TestResolveActor_UITokenOwnerIsAdmin(t *testing.T) {
	res := &IdentityResolver{
		Auth: &fakeAuth{rec: &auth.TokenRecord{
			Type: auth.TokenTypeUI, TenantID: "t1", Role: auth.RoleOwner,
		}, ok: true},
	}
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Authorization", "Bearer xyz")
	a, _ := res.ResolveActor(r)
	if a.Kind != "operator" || a.ID != "t1" || !a.IsAdmin {
		t.Fatalf("got %+v, want operator/t1/admin", a)
	}
}

func TestResolveActor_UITokenOperatorIsNotAdmin(t *testing.T) {
	res := &IdentityResolver{
		Auth: &fakeAuth{rec: &auth.TokenRecord{
			Type: auth.TokenTypeUI, TenantID: "t1", Role: auth.RoleOperator,
		}, ok: true},
	}
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Authorization", "Bearer xyz")
	a, _ := res.ResolveActor(r)
	if a.IsAdmin {
		t.Fatalf("got admin=true, want false")
	}
}

func TestResolveActor_TenantToken(t *testing.T) {
	res := &IdentityResolver{
		Auth: &fakeAuth{rec: &auth.TokenRecord{Type: auth.TokenTypeTenant, TenantID: "tn1"}, ok: true},
	}
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Authorization", "Bearer xyz")
	a, _ := res.ResolveActor(r)
	if a.Kind != "operator" || a.ID != "tn1" || a.IsAdmin {
		t.Fatalf("got %+v, want operator/tn1/!admin", a)
	}
}

func TestResolveActor_AdminToken(t *testing.T) {
	res := &IdentityResolver{
		Auth: &fakeAuth{rec: &auth.TokenRecord{Type: auth.TokenTypeAdmin, TokenID: "adminTok"}, ok: true},
	}
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Authorization", "Bearer xyz")
	a, _ := res.ResolveActor(r)
	if !a.IsAdmin {
		t.Fatalf("got admin=false, want true")
	}
}

func TestResolveActor_RevokedRejected(t *testing.T) {
	res := &IdentityResolver{
		Auth: &fakeAuth{rec: &auth.TokenRecord{Type: auth.TokenTypeAgent, Revoked: true, Name: "ops-rev"}, ok: true},
	}
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Authorization", "Bearer xyz")
	if _, err := res.ResolveActor(r); err == nil {
		t.Fatalf("want error on revoked, got nil")
	}
}

func TestResolveActor_MissingToken(t *testing.T) {
	res := &IdentityResolver{Auth: &fakeAuth{}}
	r := httptest.NewRequest("GET", "/x", nil)
	if _, err := res.ResolveActor(r); err == nil {
		t.Fatalf("want error on missing token, got nil")
	}
}

func TestResolveActor_UnknownToken(t *testing.T) {
	res := &IdentityResolver{Auth: &fakeAuth{ok: false}}
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Authorization", "Bearer abc")
	if _, err := res.ResolveActor(r); err == nil {
		t.Fatalf("want error on unknown token, got nil")
	}
}

func TestResolveActor_TokenInQueryParam(t *testing.T) {
	res := &IdentityResolver{
		Auth: &fakeAuth{rec: &auth.TokenRecord{Type: auth.TokenTypeAgent, Name: "ops"}, ok: true},
	}
	r := httptest.NewRequest("GET", "/x?token=abc", nil)
	a, err := res.ResolveActor(r)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if a.ID != "ops" {
		t.Fatalf("got %+v", a)
	}
}
