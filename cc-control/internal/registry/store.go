package registry

import (
	"database/sql"
	"encoding/json"
	"errors"
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

var ErrNotFound = errors.New("not found")

func (s *Store) Latest(name string) (*StoredSkill, error) {
	row := s.db.QueryRow(`
		SELECT name, version, body_json, author_server_id, created_at, deleted_at
		FROM skills
		WHERE name = ? AND deleted_at IS NULL
		ORDER BY version DESC LIMIT 1`, name)
	return scanStored(row)
}

func (s *Store) Get(name string, version int) (*StoredSkill, error) {
	if version == 0 {
		return s.Latest(name)
	}
	row := s.db.QueryRow(`
		SELECT name, version, body_json, author_server_id, created_at, deleted_at
		FROM skills
		WHERE name = ? AND version = ? AND deleted_at IS NULL`, name, version)
	return scanStored(row)
}

func (s *Store) List(query string) ([]Summary, error) {
	// Latest non-deleted version per name. Filter by name or description LIKE.
	q := `
		SELECT s.name, s.description, s.version, s.author_server_id, s.created_at
		FROM skills s
		JOIN (
			SELECT name, MAX(version) AS mv
			FROM skills WHERE deleted_at IS NULL
			GROUP BY name
		) m ON m.name = s.name AND m.mv = s.version
		WHERE s.deleted_at IS NULL
	`
	args := []any{}
	if query != "" {
		q += ` AND (s.name LIKE ? OR s.description LIKE ?)`
		like := "%" + query + "%"
		args = append(args, like, like)
	}
	q += ` ORDER BY s.created_at DESC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	defer rows.Close()
	var out []Summary
	for rows.Next() {
		var sm Summary
		if err := rows.Scan(&sm.Name, &sm.Description, &sm.Version, &sm.AuthorServerID, &sm.CreatedAtUnix); err != nil {
			return nil, err
		}
		out = append(out, sm)
	}
	return out, rows.Err()
}

func (s *Store) History(name string) ([]Summary, error) {
	rows, err := s.db.Query(`
		SELECT name, description, version, author_server_id, created_at
		FROM skills
		WHERE name = ? AND deleted_at IS NULL
		ORDER BY version ASC`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Summary
	for rows.Next() {
		var sm Summary
		if err := rows.Scan(&sm.Name, &sm.Description, &sm.Version, &sm.AuthorServerID, &sm.CreatedAtUnix); err != nil {
			return nil, err
		}
		out = append(out, sm)
	}
	return out, rows.Err()
}

func (s *Store) SoftDelete(name string, version int, by string) error {
	res, err := s.db.Exec(`
		UPDATE skills SET deleted_at = ?
		WHERE name = ? AND version = ? AND deleted_at IS NULL`,
		time.Now().Unix(), name, version)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanStored(row rowScanner) (*StoredSkill, error) {
	var (
		body      string
		ss        StoredSkill
		deletedAt sql.NullInt64
	)
	err := row.Scan(&ss.Name, &ss.Version, &body, &ss.AuthorServerID, &ss.CreatedAtUnix, &deletedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(body), &ss.Skill); err != nil {
		return nil, fmt.Errorf("unmarshal stored skill: %w", err)
	}
	if deletedAt.Valid {
		v := deletedAt.Int64
		ss.DeletedAtUnix = &v
	}
	return &ss, nil
}
