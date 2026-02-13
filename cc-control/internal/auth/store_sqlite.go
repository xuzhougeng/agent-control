package auth

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

func NewStoreWithSQLite(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("sqlite path required")
	}
	db, err := openSQLite(path)
	if err != nil {
		return nil, err
	}
	store := NewStore()
	store.db = db
	if err := ensureSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.loadFromSQLite(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func openSQLite(path string) (*sql.DB, error) {
	if path != ":memory:" {
		dir := filepath.Dir(path)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, err
			}
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	_, _ = db.Exec(`PRAGMA journal_mode = WAL;`)
	_, _ = db.Exec(`PRAGMA synchronous = NORMAL;`)
	_, _ = db.Exec(`PRAGMA foreign_keys = ON;`)
	_, _ = db.Exec(`PRAGMA busy_timeout = 5000;`)
	return db, nil
}

func ensureSchema(db *sql.DB) error {
	if db == nil {
		return errors.New("nil db")
	}
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS tokens (
  token_id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  type TEXT NOT NULL,
  role TEXT NOT NULL,
  created_at_ms INTEGER NOT NULL,
  revoked INTEGER NOT NULL,
  name TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_tokens_tenant ON tokens(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tokens_hash ON tokens(token_hash);
`)
	return err
}

func (s *Store) loadFromSQLite(db *sql.DB) error {
	if db == nil {
		return errors.New("nil db")
	}
	rows, err := db.Query(`
SELECT token_id, token_hash, tenant_id, type, role, created_at_ms, revoked, name
FROM tokens
`)
	if err != nil {
		return err
	}
	defer rows.Close()

	s.mu.Lock()
	defer s.mu.Unlock()
	for rows.Next() {
		var rec TokenRecord
		var tt string
		var role string
		var revokedInt int
		if err := rows.Scan(&rec.TokenID, &rec.TokenHash, &rec.TenantID, &tt, &role, &rec.CreatedAtMS, &revokedInt, &rec.Name); err != nil {
			return err
		}
		rec.Type = TokenType(tt)
		rec.Role = TokenRole(role)
		rec.Revoked = revokedInt != 0
		if rec.TokenHash == "" || rec.TokenID == "" {
			return errors.New("invalid token record in db")
		}
		if _, ok := s.byHash[rec.TokenHash]; ok {
			return fmt.Errorf("duplicate token hash in db: %s", rec.TokenHash)
		}
		if _, ok := s.byID[rec.TokenID]; ok {
			return fmt.Errorf("duplicate token id in db: %s", rec.TokenID)
		}
		copyRec := rec
		s.byHash[rec.TokenHash] = &copyRec
		s.byID[rec.TokenID] = &copyRec
	}
	return rows.Err()
}

func (s *Store) persistInsertLocked(rec *TokenRecord) error {
	if s.db == nil || rec == nil {
		return nil
	}
	revoked := 0
	if rec.Revoked {
		revoked = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO tokens (token_id, token_hash, tenant_id, type, role, created_at_ms, revoked, name)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.TokenID,
		rec.TokenHash,
		rec.TenantID,
		string(rec.Type),
		string(rec.Role),
		rec.CreatedAtMS,
		revoked,
		rec.Name,
	)
	return err
}

func (s *Store) persistRevokeByTenant(tenantID string, types []TokenType) error {
	if s.db == nil || tenantID == "" || len(types) == 0 {
		return nil
	}
	placeholders := strings.Repeat("?,", len(types))
	placeholders = strings.TrimSuffix(placeholders, ",")
	query := fmt.Sprintf(`UPDATE tokens SET revoked = 1 WHERE tenant_id = ? AND type IN (%s) AND revoked = 0`, placeholders)
	args := make([]any, 0, len(types)+1)
	args = append(args, tenantID)
	for _, tt := range types {
		args = append(args, string(tt))
	}
	_, err := s.db.Exec(query, args...)
	return err
}
