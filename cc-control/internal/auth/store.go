package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

type TokenType string

type TokenRole string

const (
	TokenTypeUI    TokenType = "ui"
	TokenTypeAgent TokenType = "agent"
	TokenTypeAdmin TokenType = "admin"
)

const (
	RoleViewer   TokenRole = "viewer"
	RoleOperator TokenRole = "operator"
	RoleOwner    TokenRole = "owner"
)

type TokenRecord struct {
	TokenID     string    `json:"token_id"`
	TokenHash   string    `json:"-"`
	TenantID    string    `json:"tenant_id"`
	Type        TokenType `json:"type"`
	Role        TokenRole `json:"role,omitempty"`
	CreatedAtMS int64     `json:"created_at_ms"`
	Revoked     bool      `json:"revoked"`
	Name        string    `json:"name,omitempty"`
}

type Store struct {
	mu     sync.RWMutex
	byHash map[string]*TokenRecord
	byID   map[string]*TokenRecord
}

func NewStore() *Store {
	return &Store{
		byHash: make(map[string]*TokenRecord),
		byID:   make(map[string]*TokenRecord),
	}
}

func ParseType(v string) (TokenType, bool) {
	switch TokenType(v) {
	case TokenTypeUI, TokenTypeAgent, TokenTypeAdmin:
		return TokenType(v), true
	default:
		return "", false
	}
}

func ParseRole(v string) (TokenRole, bool) {
	switch TokenRole(v) {
	case RoleViewer, RoleOperator, RoleOwner:
		return TokenRole(v), true
	default:
		return "", false
	}
}

func RoleAtLeast(got, need TokenRole) bool {
	return roleRank(got) >= roleRank(need)
}

func roleRank(r TokenRole) int {
	switch r {
	case RoleViewer:
		return 1
	case RoleOperator:
		return 2
	case RoleOwner:
		return 3
	default:
		return 0
	}
}

func (s *Store) CreateToken(tt TokenType, role TokenRole, tenantID, name string) (string, TokenRecord, error) {
	if !validType(tt) {
		return "", TokenRecord{}, errors.New("invalid token type")
	}
	if tt == TokenTypeUI {
		if !validRole(role) {
			return "", TokenRecord{}, errors.New("invalid token role")
		}
	} else {
		role = ""
	}
	plain, err := randomToken()
	if err != nil {
		return "", TokenRecord{}, err
	}
	rec := TokenRecord{
		TokenID:     uuid.NewString(),
		TokenHash:   HashToken(plain),
		TenantID:    tenantID,
		Type:        tt,
		Role:        role,
		CreatedAtMS: time.Now().UnixMilli(),
		Revoked:     false,
		Name:        name,
	}
	if err := s.insert(&rec); err != nil {
		return "", TokenRecord{}, err
	}
	return plain, rec, nil
}

func (s *Store) SeedToken(token string, tt TokenType, role TokenRole, tenantID, name string) (TokenRecord, error) {
	if token == "" {
		return TokenRecord{}, errors.New("token required")
	}
	if !validType(tt) {
		return TokenRecord{}, errors.New("invalid token type")
	}
	if tt == TokenTypeUI {
		if !validRole(role) {
			return TokenRecord{}, errors.New("invalid token role")
		}
	} else {
		role = ""
	}
	rec := TokenRecord{
		TokenID:     uuid.NewString(),
		TokenHash:   HashToken(token),
		TenantID:    tenantID,
		Type:        tt,
		Role:        role,
		CreatedAtMS: time.Now().UnixMilli(),
		Revoked:     false,
		Name:        name,
	}
	if err := s.insert(&rec); err != nil {
		return TokenRecord{}, err
	}
	return rec, nil
}

func (s *Store) insert(rec *TokenRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec.TokenHash == "" || rec.TokenID == "" {
		return errors.New("missing token hash or id")
	}
	if _, ok := s.byHash[rec.TokenHash]; ok {
		return errors.New("token already exists")
	}
	if _, ok := s.byID[rec.TokenID]; ok {
		return errors.New("token id already exists")
	}
	copyRec := *rec
	s.byHash[rec.TokenHash] = &copyRec
	s.byID[rec.TokenID] = &copyRec
	return nil
}

func (s *Store) Lookup(token string) (*TokenRecord, bool) {
	hash := HashToken(token)
	s.mu.RLock()
	rec := s.byHash[hash]
	s.mu.RUnlock()
	if rec == nil {
		return nil, false
	}
	copyRec := *rec
	return &copyRec, true
}

func (s *Store) RevokeToken(tokenID string) bool {
	s.mu.Lock()
	rec := s.byID[tokenID]
	if rec == nil {
		s.mu.Unlock()
		return false
	}
	rec.Revoked = true
	s.mu.Unlock()
	return true
}

func (s *Store) ListTokens(tenantID string) []TokenRecord {
	s.mu.RLock()
	out := make([]TokenRecord, 0, len(s.byID))
	for _, rec := range s.byID {
		if tenantID != "" && rec.TenantID != tenantID {
			continue
		}
		copyRec := *rec
		out = append(out, copyRec)
	}
	s.mu.RUnlock()
	return out
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func validType(tt TokenType) bool {
	switch tt {
	case TokenTypeUI, TokenTypeAgent, TokenTypeAdmin:
		return true
	default:
		return false
	}
}

func validRole(role TokenRole) bool {
	switch role {
	case RoleViewer, RoleOperator, RoleOwner:
		return true
	default:
		return false
	}
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
