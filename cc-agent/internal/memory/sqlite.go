package memory

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS sessions (
  id           TEXT PRIMARY KEY,
  created_at   INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS messages (
  id                 INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id         TEXT NOT NULL REFERENCES sessions(id),
  role               TEXT NOT NULL,
  content            TEXT,
  reasoning_content  TEXT,
  tool_calls         TEXT,
  tool_use_id        TEXT,
  tool_result        TEXT,
  tool_error         INTEGER NOT NULL DEFAULT 0,
  created_at         INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, id);
`

// alterSQL is run after schema to bring older databases up to date. SQLite
// rejects ALTER on an existing column, so we ignore those errors.
const alterSQL = `ALTER TABLE messages ADD COLUMN reasoning_content TEXT`

type SQLite struct {
	db *sql.DB
}

func OpenSQLite(path string) (*SQLite, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	// Best-effort migration; safe to fail when the column already exists.
	_, _ = db.Exec(alterSQL)
	return &SQLite{db: db}, nil
}

func (s *SQLite) EnsureSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, created_at) VALUES (?, ?) ON CONFLICT(id) DO NOTHING`,
		id, time.Now().UnixMilli(),
	)
	return err
}

func (s *SQLite) AppendMessage(ctx context.Context, sessionID string, m Message) error {
	if m.At == 0 {
		m.At = time.Now().UnixMilli()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO messages (session_id, role, content, reasoning_content, tool_calls, tool_use_id, tool_result, tool_error, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionID, m.Role, m.Content, m.ReasoningContent, m.ToolCalls, m.ToolUseID, m.ToolResult, boolToInt(m.ToolError), m.At,
	)
	return err
}

func (s *SQLite) LoadMessages(ctx context.Context, sessionID string) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, role, content, COALESCE(reasoning_content, ''), tool_calls, tool_use_id, tool_result, tool_error, created_at
  FROM messages WHERE session_id = ? ORDER BY id ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		var errInt int
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.ReasoningContent, &m.ToolCalls, &m.ToolUseID, &m.ToolResult, &errInt, &m.At); err != nil {
			return nil, err
		}
		m.ToolError = errInt != 0
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *SQLite) Sessions(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM sessions ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *SQLite) DropAfter(ctx context.Context, sessionID string, afterID int64) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM messages WHERE session_id = ? AND id > ?`,
		sessionID, afterID,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s *SQLite) Close() error { return s.db.Close() }

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
