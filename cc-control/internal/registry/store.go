package registry

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS skills (
    name             TEXT NOT NULL,
    version          INTEGER NOT NULL,
    body_json        TEXT NOT NULL,
    description      TEXT NOT NULL,
    author_server_id TEXT NOT NULL,
    created_at       INTEGER NOT NULL,
    deleted_at       INTEGER,
    PRIMARY KEY (name, version)
);
CREATE INDEX IF NOT EXISTS skills_name_idx   ON skills(name);
CREATE INDEX IF NOT EXISTS skills_author_idx ON skills(author_server_id);
`

// OpenStore opens (or creates) the sqlite registry DB at path.
func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// One writer at a time — sqlite serialises BEGIN IMMEDIATE.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Publish inserts a new version of a skill. Version is computed as
// MAX(version)+1 inside a serialised transaction so concurrent publishes
// produce strictly monotonic distinct versions.
func (s *Store) Publish(sk *Skill, authorServerID string) (int, error) {
	body, err := json.Marshal(sk)
	if err != nil {
		return 0, fmt.Errorf("marshal: %w", err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var maxV sql.NullInt64
	if err := tx.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM skills WHERE name = ?`, sk.Name).Scan(&maxV); err != nil {
		return 0, fmt.Errorf("max version: %w", err)
	}
	next := int(maxV.Int64) + 1
	now := time.Now().Unix()
	if _, err := tx.Exec(
		`INSERT INTO skills(name, version, body_json, description, author_server_id, created_at) VALUES(?, ?, ?, ?, ?, ?)`,
		sk.Name, next, string(body), sk.Description, authorServerID, now,
	); err != nil {
		return 0, fmt.Errorf("insert: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return next, nil
}
