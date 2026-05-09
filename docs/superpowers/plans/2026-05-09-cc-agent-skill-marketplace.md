# cc-agent Skill Marketplace Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a team-private skill registry hosted in cc-control, with HTTP REST endpoints, peer push/pull from cc-agent's REPL, and a "Skills" tab in cc-web for browsing.

**Architecture:** REST endpoints on cc-control backed by sqlite. Both cc-agent (over HTTP with bearer agent-token) and cc-web (over HTTP with operator session cookie) call the same surface. Skills are versioned with `MAX(version)+1` on publish; latest wins; full history kept. Reference spec: `docs/superpowers/specs/2026-05-09-cc-agent-skill-marketplace-design.md`.

**Tech Stack:** Go 1.25 (cc-control + cc-agent), `modernc.org/sqlite` (pure-Go sqlite, already a dep of cc-control), vanilla ES modules + plain HTML for cc-web (the spec mistakenly said "React" — the actual cc-web is vanilla JS modules in the same pattern as `cc-web/js/admin/`), Playwright for cc-web e2e.

---

## Spec correction

The spec's section 5.4 calls cc-web "React"; the codebase uses vanilla ES modules. This plan implements the cc-web pages using the existing pattern: one HTML file + `js/<page>/{api,render,page}.js` + `js/main-<page>.js`, mirroring `cc-web/admin.html` + `cc-web/js/admin/`. Functional behaviour matches the spec's master/detail layout exactly.

---

## File map

### New files

```
cc-control/internal/registry/skill.go                 # types
cc-control/internal/registry/validate.go              # publish-time schema validation
cc-control/internal/registry/validate_test.go
cc-control/internal/registry/store.go                 # sqlite store + schema
cc-control/internal/registry/store_test.go
cc-control/internal/registry/identity.go              # Actor resolver
cc-control/internal/registry/identity_test.go
cc-control/internal/registry/http.go                  # HTTP handlers + RegisterRoutes
cc-control/internal/registry/http_test.go
cc-agent/internal/skills/client.go                    # RegistryClient
cc-agent/internal/skills/client_test.go
cc-agent/internal/skills/load.go                      # LoadTwoPhase helper
cc-agent/internal/skills/load_test.go
cc-web/skills.html                                    # Skills tab page
cc-web/js/main-skills.js                              # entry
cc-web/js/skills/api.js                               # typed client
cc-web/js/skills/render.js                            # list + side panel renderers
cc-web/js/skills/page.js                              # initSkillsPage state machine
tests/registry_e2e/main_test.go                       # full integration smoke
tests/web-e2e/specs/skills.spec.js                    # Playwright UI smoke
```

### Modified files

```
cc-control/internal/http/server.go              # wire registry routes
cc-control/cmd/cc-control/main.go               # open registry sqlite
cc-agent/internal/config/config.go              # ControlHTTPURL field + env var
cc-agent/internal/transport/cli.go              # new slash commands
cc-agent/cmd/cc-agent/main.go                   # build + plumb RegistryClient
```

### File responsibilities (one purpose each)

- `registry/skill.go` — wire types only. No logic. Mirrors cc-agent's `Skill` shape.
- `registry/validate.go` — pure functions. Returns first violation or nil.
- `registry/store.go` — sqlite CRUD. Holds `*sql.DB`, no auth, no HTTP.
- `registry/identity.go` — bridges existing `auth.TokenRecord` → registry-level `Actor`.
- `registry/http.go` — HTTP handlers + `RegisterRoutes(mux, store, identity)`. JSON in/out, status codes, no business logic.
- `skills/client.go` — `RegistryClient` HTTP wrapper for cc-agent. Atomic file writes.
- `skills/load.go` — Two-phase loader: `LoadDir(local)` then `LoadDir(team)` so team wins deterministically.
- `cc-web/js/skills/api.js` — fetch wrappers, typed return shapes.
- `cc-web/js/skills/render.js` — pure DOM rendering. No state.
- `cc-web/js/skills/page.js` — state + event wiring. Calls render.js.

---

## Phase A — Registry data layer (cc-control)

### Task 1: Skeleton + types

**Files:**
- Create: `cc-control/internal/registry/skill.go`

- [ ] **Step 1: Create the package + Skill type**

```go
// Package registry implements cc-control's team-private skill marketplace.
// It exposes HTTP endpoints for cc-agent and cc-web to publish, install, list,
// and browse skill kernels. Storage is a sqlite database independent of the
// existing session store.
package registry

// Skill is the wire format for a skill kernel. Mirrors cc-agent's
// internal/skills.Skill so JSON crosses the boundary unchanged.
type Skill struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Prompt      string   `json:"prompt"`
	Tools       []string `json:"tools"`
	Examples    []string `json:"examples"`
}

// StoredSkill is what the store returns: a Skill plus registry metadata.
type StoredSkill struct {
	Skill
	Version          int    `json:"version"`
	AuthorServerID   string `json:"author_server_id"`
	CreatedAtUnix    int64  `json:"created_at_unix"`
	DeletedAtUnix    *int64 `json:"deleted_at_unix,omitempty"`
}

// Summary is a list-row shape: enough to render one row, no full body.
type Summary struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	Version          int    `json:"version"`
	AuthorServerID   string `json:"author_server_id"`
	CreatedAtUnix    int64  `json:"created_at_unix"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd cc-control && go build ./internal/registry/...`
Expected: clean build.

- [ ] **Step 3: Commit**

```bash
git add cc-control/internal/registry/skill.go
git commit -m "feat(registry): scaffold types for skill marketplace"
```

---

### Task 2: Validation

**Files:**
- Create: `cc-control/internal/registry/validate.go`
- Test: `cc-control/internal/registry/validate_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package registry

import "testing"

func TestValidate(t *testing.T) {
	knownTools := []string{"bash", "read", "write", "grep", "glob", "sysinfo", "proclist", "logtail"}
	cases := []struct {
		name      string
		s         Skill
		wantField string // empty = expect nil error
	}{
		{"happy path", Skill{Name: "nginx-triage", Prompt: "x", Tools: []string{"bash"}}, ""},
		{"empty name", Skill{Name: "", Prompt: "x"}, "name"},
		{"bad name regex (uppercase)", Skill{Name: "Nginx", Prompt: "x"}, "name"},
		{"bad name regex (starts digit)", Skill{Name: "1foo", Prompt: "x"}, "name"},
		{"too long name", Skill{Name: string(makeBytes('a', 80)), Prompt: "x"}, "name"},
		{"empty prompt", Skill{Name: "ok", Prompt: ""}, "prompt"},
		{"prompt too big", Skill{Name: "ok", Prompt: string(makeBytes('a', 33*1024))}, "prompt"},
		{"unknown tool", Skill{Name: "ok", Prompt: "x", Tools: []string{"kubectl"}}, "tools"},
		{"too many examples", Skill{Name: "ok", Prompt: "x", Examples: makeStrs(11)}, "examples"},
		{"example too long", Skill{Name: "ok", Prompt: "x", Examples: []string{string(makeBytes('a', 1025))}}, "examples"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(&tc.s, knownTools)
			if tc.wantField == "" {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want error on field %q, got nil", tc.wantField)
			}
			ve, ok := err.(*ValidationError)
			if !ok {
				t.Fatalf("want *ValidationError, got %T", err)
			}
			if ve.Field != tc.wantField {
				t.Fatalf("want field %q, got %q", tc.wantField, ve.Field)
			}
		})
	}
}

func makeBytes(c byte, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = c
	}
	return b
}

func makeStrs(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "x"
	}
	return out
}
```

- [ ] **Step 2: Run test (expect compile failure)**

Run: `cd cc-control && go test ./internal/registry/ -run Validate -v`
Expected: FAIL — `Validate` undefined.

- [ ] **Step 3: Implement validate.go**

```go
package registry

import (
	"fmt"
	"regexp"
)

const (
	maxPromptBytes  = 32 * 1024
	maxExampleBytes = 1024
	maxExamples     = 10
)

var nameRE = regexp.MustCompile(`^[a-z][a-z0-9-]{1,63}$`)

type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid %s: %s", e.Field, e.Reason)
}

// Validate checks a Skill for publishability against known tool names.
// Returns nil on success, *ValidationError on the first violation.
func Validate(s *Skill, knownTools []string) error {
	if !nameRE.MatchString(s.Name) {
		return &ValidationError{Field: "name", Reason: "must match ^[a-z][a-z0-9-]{1,63}$"}
	}
	if s.Prompt == "" {
		return &ValidationError{Field: "prompt", Reason: "must be non-empty"}
	}
	if len(s.Prompt) > maxPromptBytes {
		return &ValidationError{Field: "prompt", Reason: fmt.Sprintf("exceeds %d bytes", maxPromptBytes)}
	}
	known := map[string]bool{}
	for _, t := range knownTools {
		known[t] = true
	}
	for _, t := range s.Tools {
		if !known[t] {
			return &ValidationError{Field: "tools", Reason: fmt.Sprintf("unknown tool %q", t)}
		}
	}
	if len(s.Examples) > maxExamples {
		return &ValidationError{Field: "examples", Reason: fmt.Sprintf("more than %d examples", maxExamples)}
	}
	for i, e := range s.Examples {
		if len(e) > maxExampleBytes {
			return &ValidationError{Field: "examples", Reason: fmt.Sprintf("example %d exceeds %d bytes", i, maxExampleBytes)}
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test (expect pass)**

Run: `cd cc-control && go test ./internal/registry/ -run Validate -v`
Expected: PASS for all 10 cases.

- [ ] **Step 5: Commit**

```bash
git add cc-control/internal/registry/validate.go cc-control/internal/registry/validate_test.go
git commit -m "feat(registry): publish-time skill validation"
```

---

### Task 3: Store — schema + Publish (with concurrency test)

**Files:**
- Create: `cc-control/internal/registry/store.go`
- Test: `cc-control/internal/registry/store_test.go`

- [ ] **Step 1: Write failing tests**

```go
package registry

import (
	"path/filepath"
	"sync"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	st, err := OpenStore(filepath.Join(dir, "registry.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestPublish_AssignsMonotonicVersions(t *testing.T) {
	st := newTestStore(t)
	v1, err := st.Publish(&Skill{Name: "x", Prompt: "p"}, "ops-01")
	if err != nil {
		t.Fatalf("publish 1: %v", err)
	}
	if v1 != 1 {
		t.Fatalf("first version = %d, want 1", v1)
	}
	v2, err := st.Publish(&Skill{Name: "x", Prompt: "p2"}, "ops-02")
	if err != nil {
		t.Fatalf("publish 2: %v", err)
	}
	if v2 != 2 {
		t.Fatalf("second version = %d, want 2", v2)
	}
}

func TestPublish_ConcurrentRaceProducesDistinctVersions(t *testing.T) {
	st := newTestStore(t)
	const N = 20
	var wg sync.WaitGroup
	versions := make(chan int, N)
	errs := make(chan error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := st.Publish(&Skill{Name: "x", Prompt: "p"}, "ops-X")
			if err != nil {
				errs <- err
				return
			}
			versions <- v
		}()
	}
	wg.Wait()
	close(versions)
	close(errs)
	for e := range errs {
		t.Fatalf("publish error: %v", e)
	}
	seen := map[int]bool{}
	for v := range versions {
		if seen[v] {
			t.Fatalf("duplicate version %d", v)
		}
		seen[v] = true
	}
	if len(seen) != N {
		t.Fatalf("got %d distinct versions, want %d", len(seen), N)
	}
}
```

- [ ] **Step 2: Run tests (expect compile failure)**

Run: `cd cc-control && go test ./internal/registry/ -run Publish -v`
Expected: FAIL — `OpenStore`/`Publish` undefined.

- [ ] **Step 3: Implement store.go (schema + Publish only)**

```go
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
// MAX(version)+1 inside a BEGIN IMMEDIATE transaction so concurrent publishes
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
	if _, err := tx.Exec("BEGIN IMMEDIATE"); err != nil {
		// modernc.org/sqlite doesn't accept a second BEGIN inside a tx; the
		// db.Begin() above already opens an implicit DEFERRED tx. We escalate
		// by writing a dummy lock row then DELETEing — sqlite's default mode
		// upgrades the tx to write-locked once a write occurs. Simpler: rely
		// on db.Begin() + the SetMaxOpenConns(1) serialisation.
	}
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
```

- [ ] **Step 4: Run tests (expect pass)**

Run: `cd cc-control && go test ./internal/registry/ -run Publish -v -race`
Expected: both PASS, no race detector warnings.

- [ ] **Step 5: Commit**

```bash
git add cc-control/internal/registry/store.go cc-control/internal/registry/store_test.go
git commit -m "feat(registry): sqlite store with monotonic-version publish"
```

---

### Task 4: Store — Get, Latest, List, History, SoftDelete

**Files:**
- Modify: `cc-control/internal/registry/store.go` (add methods)
- Modify: `cc-control/internal/registry/store_test.go` (add tests)

- [ ] **Step 1: Append failing tests**

```go
func TestLatest_ReturnsHighestNotDeleted(t *testing.T) {
	st := newTestStore(t)
	mustPublish(t, st, &Skill{Name: "x", Prompt: "p1"}, "a")
	mustPublish(t, st, &Skill{Name: "x", Prompt: "p2"}, "a")
	mustPublish(t, st, &Skill{Name: "x", Prompt: "p3"}, "a")
	got, err := st.Latest("x")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if got.Version != 3 || got.Prompt != "p3" {
		t.Fatalf("got version=%d prompt=%q, want 3/p3", got.Version, got.Prompt)
	}
}

func TestGet_Versioned(t *testing.T) {
	st := newTestStore(t)
	mustPublish(t, st, &Skill{Name: "x", Prompt: "p1"}, "a")
	mustPublish(t, st, &Skill{Name: "x", Prompt: "p2"}, "a")
	got, err := st.Get("x", 1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Prompt != "p1" {
		t.Fatalf("Get(x,1).Prompt=%q want p1", got.Prompt)
	}
}

func TestGet_NotFound(t *testing.T) {
	st := newTestStore(t)
	_, err := st.Get("nope", 0)
	if err != ErrNotFound {
		t.Fatalf("Get(nope) err = %v, want ErrNotFound", err)
	}
}

func TestList_FiltersByQuery(t *testing.T) {
	st := newTestStore(t)
	mustPublish(t, st, &Skill{Name: "nginx-triage", Prompt: "p", Description: "Triage nginx"}, "a")
	mustPublish(t, st, &Skill{Name: "k8s-debug", Prompt: "p", Description: "Debug k8s"}, "a")
	all, err := st.List("")
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List() len=%d, want 2", len(all))
	}
	hits, err := st.List("nginx")
	if err != nil {
		t.Fatalf("List(nginx): %v", err)
	}
	if len(hits) != 1 || hits[0].Name != "nginx-triage" {
		t.Fatalf("List(nginx) = %+v, want [nginx-triage]", hits)
	}
}

func TestHistory_AllVersionsOldestFirst(t *testing.T) {
	st := newTestStore(t)
	mustPublish(t, st, &Skill{Name: "x", Prompt: "p1"}, "a")
	mustPublish(t, st, &Skill{Name: "x", Prompt: "p2"}, "b")
	mustPublish(t, st, &Skill{Name: "x", Prompt: "p3"}, "c")
	hist, err := st.History("x")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 3 {
		t.Fatalf("len=%d, want 3", len(hist))
	}
	for i, h := range hist {
		if h.Version != i+1 {
			t.Fatalf("hist[%d].Version=%d", i, h.Version)
		}
	}
}

func TestSoftDelete_HidesFromListAndLatest(t *testing.T) {
	st := newTestStore(t)
	mustPublish(t, st, &Skill{Name: "x", Prompt: "p1"}, "a")
	mustPublish(t, st, &Skill{Name: "x", Prompt: "p2"}, "a")
	if err := st.SoftDelete("x", 2, "admin"); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	got, err := st.Latest("x")
	if err != nil {
		t.Fatalf("Latest after delete: %v", err)
	}
	if got.Version != 1 {
		t.Fatalf("Latest.Version=%d, want 1", got.Version)
	}
	hits, err := st.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(hits) != 1 || hits[0].Version != 1 {
		t.Fatalf("List = %+v, want one row at v1", hits)
	}
}

func mustPublish(t *testing.T, st *Store, s *Skill, author string) int {
	t.Helper()
	v, err := st.Publish(s, author)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	return v
}
```

- [ ] **Step 2: Run tests (expect compile failure)**

Run: `cd cc-control && go test ./internal/registry/ -v`
Expected: FAIL — `Latest`, `Get`, `List`, `History`, `SoftDelete`, `ErrNotFound` undefined.

- [ ] **Step 3: Append the methods to store.go**

```go
import "errors"

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
		body         string
		ss           StoredSkill
		deletedAt    sql.NullInt64
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
```

- [ ] **Step 4: Run tests (expect pass)**

Run: `cd cc-control && go test ./internal/registry/ -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add cc-control/internal/registry/store.go cc-control/internal/registry/store_test.go
git commit -m "feat(registry): list/get/history/soft-delete on store"
```

---

### Task 5: Identity resolver

**Files:**
- Create: `cc-control/internal/registry/identity.go`
- Test: `cc-control/internal/registry/identity_test.go`

- [ ] **Step 1: Write failing tests**

```go
package registry

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"cc-control/internal/auth"
)

type fakeAuth struct {
	rec *auth.TokenRecord
	err error
}

func (f *fakeAuth) Resolve(token string) (*auth.TokenRecord, error) {
	return f.rec, f.err
}

func TestResolveActor_AgentBearerToken(t *testing.T) {
	res := &IdentityResolver{
		Auth: &fakeAuth{rec: &auth.TokenRecord{Kind: "agent", AgentServerID: "ops-01"}},
	}
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Authorization", "Bearer abc")
	a, err := res.ResolveActor(r)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if a.Kind != "agent" || a.ID != "ops-01" {
		t.Fatalf("got %+v, want agent/ops-01", a)
	}
}

func TestResolveActor_OperatorTenantToken(t *testing.T) {
	res := &IdentityResolver{
		Auth: &fakeAuth{rec: &auth.TokenRecord{Kind: "tenant", TenantUserID: "alice"}},
	}
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Authorization", "Bearer xyz")
	a, err := res.ResolveActor(r)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if a.Kind != "operator" || a.ID != "alice" {
		t.Fatalf("got %+v, want operator/alice", a)
	}
}

func TestResolveActor_AdminFlagFromTokenRecord(t *testing.T) {
	res := &IdentityResolver{
		Auth: &fakeAuth{rec: &auth.TokenRecord{Kind: "tenant", TenantUserID: "alice", IsAdmin: true}},
	}
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Authorization", "Bearer xyz")
	a, _ := res.ResolveActor(r)
	if !a.IsAdmin {
		t.Fatalf("IsAdmin=false, want true")
	}
}

func TestResolveActor_MissingToken(t *testing.T) {
	res := &IdentityResolver{Auth: &fakeAuth{}}
	r := httptest.NewRequest("GET", "/x", nil)
	if _, err := res.ResolveActor(r); err == nil {
		t.Fatalf("want error, got nil")
	}
}

var _ http.Handler = http.NotFoundHandler() // ensure net/http is used
```

> **Note for implementer:** `auth.TokenRecord` may not currently have `Kind`, `AgentServerID`, `TenantUserID`, `IsAdmin` fields. Check `cc-control/internal/auth/store.go` first. If those names differ, adapt the test and `identity.go` to the actual field names — but keep the *Actor* shape (Kind/ID/IsAdmin) unchanged. The bridge is the whole point of `identity.go`: shield registry code from auth-package churn.

- [ ] **Step 2: Read auth.TokenRecord and adjust the tests**

Run: `grep -n "type TokenRecord" cc-control/internal/auth/store.go`
Use the actual field names there. Update both the fake and the real `identity.go` accordingly. The test cases above describe the *behaviour* the `IdentityResolver` must have, not specific field names.

- [ ] **Step 3: Run tests (expect compile failure)**

Run: `cd cc-control && go test ./internal/registry/ -run Resolve -v`
Expected: FAIL — `IdentityResolver` undefined.

- [ ] **Step 4: Implement identity.go**

```go
package registry

import (
	"errors"
	"net/http"
	"strings"

	"cc-control/internal/auth"
)

type Actor struct {
	Kind    string // "agent" | "operator"
	ID      string // server_id (agent) or user_id (operator)
	IsAdmin bool
}

// AuthBackend is what we need from the existing auth.Store. Defining it as an
// interface lets identity_test.go inject a fake.
type AuthBackend interface {
	Resolve(token string) (*auth.TokenRecord, error)
}

type IdentityResolver struct {
	Auth AuthBackend
}

func (r *IdentityResolver) ResolveActor(req *http.Request) (Actor, error) {
	tok := bearer(req)
	if tok == "" {
		return Actor{}, errors.New("missing bearer token")
	}
	rec, err := r.Auth.Resolve(tok)
	if err != nil || rec == nil {
		return Actor{}, errors.New("invalid token")
	}
	// Map auth kind → registry actor kind. Adjust field names below to match
	// the actual auth.TokenRecord shape after Step 2.
	switch rec.Kind {
	case "agent":
		return Actor{Kind: "agent", ID: rec.AgentServerID}, nil
	case "tenant", "ui":
		return Actor{Kind: "operator", ID: rec.TenantUserID, IsAdmin: rec.IsAdmin}, nil
	default:
		return Actor{}, errors.New("unsupported token kind")
	}
}

func bearer(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}
```

- [ ] **Step 5: Run tests (expect pass after Step 2 adjustments)**

Run: `cd cc-control && go test ./internal/registry/ -run Resolve -v`
Expected: PASS for all four resolve cases.

- [ ] **Step 6: Commit**

```bash
git add cc-control/internal/registry/identity.go cc-control/internal/registry/identity_test.go
git commit -m "feat(registry): identity resolver bridging auth.TokenRecord"
```

---

## Phase B — Registry HTTP layer

### Task 6: HTTP — POST publish

**Files:**
- Create: `cc-control/internal/registry/http.go`
- Test: `cc-control/internal/registry/http_test.go`

- [ ] **Step 1: Write failing test for the publish endpoint**

```go
package registry

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func newTestServer(t *testing.T, actor Actor) (*httptest.Server, *Store) {
	t.Helper()
	dir := t.TempDir()
	st, err := OpenStore(filepath.Join(dir, "registry.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	mux := http.NewServeMux()
	RegisterRoutes(mux, &RouteDeps{
		Store:      st,
		KnownTools: []string{"bash", "read", "write", "grep", "glob", "sysinfo", "proclist", "logtail"},
		Identity:   stubIdentity{actor: actor},
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, st
}

type stubIdentity struct{ actor Actor }

func (s stubIdentity) ResolveActor(*http.Request) (Actor, error) { return s.actor, nil }

func TestPublishHandler_Success(t *testing.T) {
	srv, _ := newTestServer(t, Actor{Kind: "agent", ID: "ops-01"})
	body, _ := json.Marshal(Skill{Name: "nginx-triage", Prompt: "p", Tools: []string{"bash"}})
	resp, err := http.Post(srv.URL+"/api/registry/skills", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		b, _ := json.Marshal(resp.Status)
		t.Fatalf("status %d: %s", resp.StatusCode, b)
	}
	var got struct{ Version int }
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got.Version != 1 {
		t.Fatalf("version=%d, want 1", got.Version)
	}
}

func TestPublishHandler_ValidationError(t *testing.T) {
	srv, _ := newTestServer(t, Actor{Kind: "agent", ID: "ops-01"})
	body, _ := json.Marshal(Skill{Name: "BAD UPPER", Prompt: "p"})
	resp, _ := http.Post(srv.URL+"/api/registry/skills", "application/json", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status=%d, want 400", resp.StatusCode)
	}
	var e struct{ Code, Field string }
	_ = json.NewDecoder(resp.Body).Decode(&e)
	if e.Code != "invalid_skill" || e.Field != "name" {
		t.Fatalf("body=%+v", e)
	}
}

func TestPublishHandler_RequiresAuth(t *testing.T) {
	dir := t.TempDir()
	st, _ := OpenStore(filepath.Join(dir, "r.db"))
	defer st.Close()
	mux := http.NewServeMux()
	RegisterRoutes(mux, &RouteDeps{
		Store:      st,
		KnownTools: []string{"bash"},
		Identity:   stubIdentityErr{},
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	body := strings.NewReader(`{"name":"x","prompt":"p"}`)
	resp, _ := http.Post(srv.URL+"/api/registry/skills", "application/json", body)
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status=%d, want 401", resp.StatusCode)
	}
}

type stubIdentityErr struct{}

func (stubIdentityErr) ResolveActor(*http.Request) (Actor, error) {
	return Actor{}, errAuth("nope")
}

type errAuth string

func (e errAuth) Error() string { return string(e) }
```

- [ ] **Step 2: Run tests (expect compile failure)**

Run: `cd cc-control && go test ./internal/registry/ -run PublishHandler -v`
Expected: FAIL — `RegisterRoutes`, `RouteDeps`, `IdentityProvider` undefined.

- [ ] **Step 3: Implement http.go for the POST endpoint only**

```go
package registry

import (
	"encoding/json"
	"net/http"
)

type IdentityProvider interface {
	ResolveActor(*http.Request) (Actor, error)
}

type RouteDeps struct {
	Store      *Store
	KnownTools []string
	Identity   IdentityProvider
}

// RegisterRoutes wires registry HTTP handlers onto mux. Caller is responsible
// for the path prefix; routes are registered under /api/registry/.
func RegisterRoutes(mux *http.ServeMux, d *RouteDeps) {
	mux.HandleFunc("/api/registry/skills", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			d.handlePublish(w, r)
		case http.MethodGet:
			http.Error(w, "list not implemented", http.StatusNotImplemented)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func (d *RouteDeps) handlePublish(w http.ResponseWriter, r *http.Request) {
	actor, err := d.Identity.ResolveActor(r)
	if err != nil {
		writeJSON(w, 401, errBody{Code: "unauth"})
		return
	}
	var sk Skill
	if err := json.NewDecoder(r.Body).Decode(&sk); err != nil {
		writeJSON(w, 400, errBody{Code: "bad_json", Reason: err.Error()})
		return
	}
	if err := Validate(&sk, d.KnownTools); err != nil {
		ve, _ := err.(*ValidationError)
		writeJSON(w, 400, errBody{Code: "invalid_skill", Field: ve.Field, Reason: ve.Reason})
		return
	}
	authorID := actor.ID
	v, err := d.Store.Publish(&sk, authorID)
	if err != nil {
		writeJSON(w, 500, errBody{Code: "store", Reason: err.Error()})
		return
	}
	writeJSON(w, 201, map[string]any{"name": sk.Name, "version": v, "author_server_id": authorID})
}

type errBody struct {
	Code   string `json:"code"`
	Field  string `json:"field,omitempty"`
	Reason string `json:"reason,omitempty"`
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
```

- [ ] **Step 4: Run tests (expect pass)**

Run: `cd cc-control && go test ./internal/registry/ -run PublishHandler -v`
Expected: all 3 PASS.

- [ ] **Step 5: Commit**

```bash
git add cc-control/internal/registry/http.go cc-control/internal/registry/http_test.go
git commit -m "feat(registry): POST /api/registry/skills (publish)"
```

---

### Task 7: HTTP — GET list, GET one, GET history

**Files:**
- Modify: `cc-control/internal/registry/http.go`
- Modify: `cc-control/internal/registry/http_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestList_Empty(t *testing.T) {
	srv, _ := newTestServer(t, Actor{Kind: "operator", ID: "alice"})
	resp, _ := http.Get(srv.URL + "/api/registry/skills")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var got []Summary
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if len(got) != 0 {
		t.Fatalf("got %d, want 0", len(got))
	}
}

func TestList_AfterPublish(t *testing.T) {
	srv, st := newTestServer(t, Actor{Kind: "agent", ID: "ops-01"})
	_, _ = st.Publish(&Skill{Name: "x", Prompt: "p", Description: "demo"}, "ops-01")
	resp, _ := http.Get(srv.URL + "/api/registry/skills")
	defer resp.Body.Close()
	var got []Summary
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if len(got) != 1 || got[0].Name != "x" {
		t.Fatalf("got %+v", got)
	}
}

func TestGetOne_Latest(t *testing.T) {
	srv, st := newTestServer(t, Actor{Kind: "agent", ID: "ops-01"})
	_, _ = st.Publish(&Skill{Name: "x", Prompt: "p1"}, "ops-01")
	_, _ = st.Publish(&Skill{Name: "x", Prompt: "p2"}, "ops-01")
	resp, _ := http.Get(srv.URL + "/api/registry/skills/x")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var got StoredSkill
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got.Version != 2 || got.Prompt != "p2" {
		t.Fatalf("got %+v", got)
	}
}

func TestGetOne_PinnedVersion(t *testing.T) {
	srv, st := newTestServer(t, Actor{Kind: "agent", ID: "ops-01"})
	_, _ = st.Publish(&Skill{Name: "x", Prompt: "p1"}, "ops-01")
	_, _ = st.Publish(&Skill{Name: "x", Prompt: "p2"}, "ops-01")
	resp, _ := http.Get(srv.URL + "/api/registry/skills/x?version=1")
	defer resp.Body.Close()
	var got StoredSkill
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got.Prompt != "p1" {
		t.Fatalf("got %+v", got)
	}
}

func TestGetOne_NotFound(t *testing.T) {
	srv, _ := newTestServer(t, Actor{Kind: "agent", ID: "ops-01"})
	resp, _ := http.Get(srv.URL + "/api/registry/skills/missing")
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("status=%d, want 404", resp.StatusCode)
	}
}

func TestHistory(t *testing.T) {
	srv, st := newTestServer(t, Actor{Kind: "agent", ID: "ops-01"})
	_, _ = st.Publish(&Skill{Name: "x", Prompt: "p1"}, "ops-01")
	_, _ = st.Publish(&Skill{Name: "x", Prompt: "p2"}, "ops-01")
	resp, _ := http.Get(srv.URL + "/api/registry/skills/x/history")
	defer resp.Body.Close()
	var hist []Summary
	_ = json.NewDecoder(resp.Body).Decode(&hist)
	if len(hist) != 2 {
		t.Fatalf("got %d, want 2", len(hist))
	}
}
```

- [ ] **Step 2: Run tests (expect failure)**

Run: `cd cc-control && go test ./internal/registry/ -run "List|GetOne|History" -v`
Expected: FAIL — handlers not yet implemented.

- [ ] **Step 3: Replace `RegisterRoutes` + add new handlers**

```go
func RegisterRoutes(mux *http.ServeMux, d *RouteDeps) {
	mux.HandleFunc("/api/registry/skills", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			d.handlePublish(w, r)
		case http.MethodGet:
			d.handleList(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/registry/skills/", func(w http.ResponseWriter, r *http.Request) {
		// path: /api/registry/skills/<name>            (GET / DELETE)
		// path: /api/registry/skills/<name>/history    (GET)
		// path: /api/registry/skills/<name>/<version>  (DELETE)
		d.handleSubroutes(w, r)
	})
	mux.HandleFunc("/api/registry/install_request", d.handleInstallRequest)
}

func (d *RouteDeps) handleList(w http.ResponseWriter, r *http.Request) {
	if _, err := d.Identity.ResolveActor(r); err != nil {
		writeJSON(w, 401, errBody{Code: "unauth"})
		return
	}
	q := r.URL.Query().Get("q")
	out, err := d.Store.List(q)
	if err != nil {
		writeJSON(w, 500, errBody{Code: "store", Reason: err.Error()})
		return
	}
	if out == nil {
		out = []Summary{}
	}
	writeJSON(w, 200, out)
}

func (d *RouteDeps) handleSubroutes(w http.ResponseWriter, r *http.Request) {
	actor, err := d.Identity.ResolveActor(r)
	if err != nil {
		writeJSON(w, 401, errBody{Code: "unauth"})
		return
	}
	rest := r.URL.Path[len("/api/registry/skills/"):]
	parts := splitPath(rest)
	switch len(parts) {
	case 1:
		// /api/registry/skills/<name>
		switch r.Method {
		case http.MethodGet:
			d.getOne(w, r, parts[0])
		default:
			http.Error(w, "method not allowed", 405)
		}
	case 2:
		switch parts[1] {
		case "history":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", 405)
				return
			}
			d.getHistory(w, r, parts[0])
		default:
			// /api/registry/skills/<name>/<version> — DELETE only
			if r.Method != http.MethodDelete {
				http.Error(w, "method not allowed", 405)
				return
			}
			d.deleteVersion(w, r, parts[0], parts[1], actor)
		}
	default:
		http.Error(w, "not found", 404)
	}
}

func splitPath(p string) []string {
	out := []string{}
	cur := ""
	for _, c := range p {
		if c == '/' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(c)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func (d *RouteDeps) getOne(w http.ResponseWriter, r *http.Request, name string) {
	versionStr := r.URL.Query().Get("version")
	version := 0
	if versionStr != "" {
		var err error
		version, err = parseInt(versionStr)
		if err != nil {
			writeJSON(w, 400, errBody{Code: "bad_version", Reason: err.Error()})
			return
		}
	}
	got, err := d.Store.Get(name, version)
	if err == ErrNotFound {
		writeJSON(w, 404, errBody{Code: "not_found"})
		return
	}
	if err != nil {
		writeJSON(w, 500, errBody{Code: "store", Reason: err.Error()})
		return
	}
	writeJSON(w, 200, got)
}

func (d *RouteDeps) getHistory(w http.ResponseWriter, _ *http.Request, name string) {
	hist, err := d.Store.History(name)
	if err != nil {
		writeJSON(w, 500, errBody{Code: "store", Reason: err.Error()})
		return
	}
	if hist == nil {
		hist = []Summary{}
	}
	writeJSON(w, 200, hist)
}

func parseInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number: %q", s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
```

> Note the new import: `"fmt"` is already imported. `deleteVersion` and `handleInstallRequest` are stubbed in the next two tasks; for now, leave them as no-op `func (d *RouteDeps) deleteVersion(http.ResponseWriter, *http.Request, string, string, Actor) {}` and `func (d *RouteDeps) handleInstallRequest(http.ResponseWriter, *http.Request) {}` so the package compiles.

- [ ] **Step 4: Add stub handlers so the package compiles**

```go
func (d *RouteDeps) deleteVersion(w http.ResponseWriter, _ *http.Request, _ string, _ string, _ Actor) {
	http.Error(w, "delete not implemented", http.StatusNotImplemented)
}

func (d *RouteDeps) handleInstallRequest(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "install_request not implemented", http.StatusNotImplemented)
}
```

- [ ] **Step 5: Run tests**

Run: `cd cc-control && go test ./internal/registry/ -run "List|GetOne|History|Publish" -v`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add cc-control/internal/registry/http.go cc-control/internal/registry/http_test.go
git commit -m "feat(registry): GET list / get-one / history endpoints"
```

---

### Task 8: HTTP — DELETE (admin-only)

**Files:**
- Modify: `cc-control/internal/registry/http.go`
- Modify: `cc-control/internal/registry/http_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestDelete_AdminCanSoftDelete(t *testing.T) {
	srv, st := newTestServer(t, Actor{Kind: "operator", ID: "admin", IsAdmin: true})
	_, _ = st.Publish(&Skill{Name: "x", Prompt: "p"}, "ops-01")
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/registry/skills/x/1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("status=%d, want 204", resp.StatusCode)
	}
	if _, err := st.Latest("x"); err != ErrNotFound {
		t.Fatalf("Latest after delete: %v", err)
	}
}

func TestDelete_NonAdminForbidden(t *testing.T) {
	srv, st := newTestServer(t, Actor{Kind: "operator", ID: "alice", IsAdmin: false})
	_, _ = st.Publish(&Skill{Name: "x", Prompt: "p"}, "ops-01")
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/registry/skills/x/1", nil)
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("status=%d, want 403", resp.StatusCode)
	}
}

func TestDelete_AgentForbidden(t *testing.T) {
	srv, st := newTestServer(t, Actor{Kind: "agent", ID: "ops-01"})
	_, _ = st.Publish(&Skill{Name: "x", Prompt: "p"}, "ops-01")
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/registry/skills/x/1", nil)
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("status=%d, want 403 (agents can't delete)", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run tests (expect failure)**

Run: `cd cc-control && go test ./internal/registry/ -run Delete -v`
Expected: FAIL — handler returns 501.

- [ ] **Step 3: Replace `deleteVersion`**

```go
func (d *RouteDeps) deleteVersion(w http.ResponseWriter, _ *http.Request, name, versionStr string, actor Actor) {
	if actor.Kind != "operator" || !actor.IsAdmin {
		writeJSON(w, 403, errBody{Code: "forbidden"})
		return
	}
	version, err := parseInt(versionStr)
	if err != nil {
		writeJSON(w, 400, errBody{Code: "bad_version", Reason: err.Error()})
		return
	}
	if err := d.Store.SoftDelete(name, version, actor.ID); err == ErrNotFound {
		writeJSON(w, 404, errBody{Code: "not_found"})
		return
	} else if err != nil {
		writeJSON(w, 500, errBody{Code: "store", Reason: err.Error()})
		return
	}
	w.WriteHeader(204)
}
```

- [ ] **Step 4: Run tests (expect pass)**

Run: `cd cc-control && go test ./internal/registry/ -run Delete -v`
Expected: all 3 PASS.

- [ ] **Step 5: Commit**

```bash
git add cc-control/internal/registry/http.go cc-control/internal/registry/http_test.go
git commit -m "feat(registry): DELETE soft-delete endpoint (admin-only)"
```

---

### Task 9: HTTP — install_request

**Files:**
- Modify: `cc-control/internal/registry/http.go`
- Modify: `cc-control/internal/registry/http_test.go`

This endpoint sends `install_skill_request` to a target agent over cc-control's existing chat WS. We'll define the wire shape and the handler; actual WS delivery is delegated to a `Notifier` interface that the integration layer (Task 11) provides.

- [ ] **Step 1: Append failing tests**

```go
type fakeNotifier struct {
	calls []NotifyCall
}

type NotifyCall struct {
	TargetAgentID string
	Skill         StoredSkill
}

func (f *fakeNotifier) NotifyInstall(target string, sk StoredSkill) error {
	f.calls = append(f.calls, NotifyCall{TargetAgentID: target, Skill: sk})
	return nil
}

func newTestServerWithNotifier(t *testing.T, actor Actor, notif InstallNotifier) (*httptest.Server, *Store) {
	t.Helper()
	dir := t.TempDir()
	st, err := OpenStore(filepath.Join(dir, "registry.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	mux := http.NewServeMux()
	RegisterRoutes(mux, &RouteDeps{
		Store:      st,
		KnownTools: []string{"bash"},
		Identity:   stubIdentity{actor: actor},
		Installer:  notif,
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, st
}

func TestInstallRequest_OperatorTriggersNotify(t *testing.T) {
	notif := &fakeNotifier{}
	srv, st := newTestServerWithNotifier(t, Actor{Kind: "operator", ID: "alice"}, notif)
	_, _ = st.Publish(&Skill{Name: "x", Prompt: "p"}, "ops-01")
	body, _ := json.Marshal(map[string]any{"name": "x", "version": 0, "target_agent_id": "ops-02"})
	resp, err := http.Post(srv.URL+"/api/registry/install_request", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 202 {
		t.Fatalf("status=%d, want 202", resp.StatusCode)
	}
	if len(notif.calls) != 1 || notif.calls[0].TargetAgentID != "ops-02" || notif.calls[0].Skill.Name != "x" {
		t.Fatalf("notifier calls=%+v", notif.calls)
	}
}

func TestInstallRequest_AgentForbidden(t *testing.T) {
	notif := &fakeNotifier{}
	srv, st := newTestServerWithNotifier(t, Actor{Kind: "agent", ID: "ops-01"}, notif)
	_, _ = st.Publish(&Skill{Name: "x", Prompt: "p"}, "ops-01")
	body := strings.NewReader(`{"name":"x","target_agent_id":"ops-02"}`)
	resp, _ := http.Post(srv.URL+"/api/registry/install_request", "application/json", body)
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("status=%d, want 403 (agents can't push installs to other agents)", resp.StatusCode)
	}
	if len(notif.calls) != 0 {
		t.Fatalf("expected no notifier calls, got %+v", notif.calls)
	}
}
```

- [ ] **Step 2: Run tests (expect failure)**

Run: `cd cc-control && go test ./internal/registry/ -run InstallRequest -v`
Expected: FAIL — `InstallNotifier`, `Installer` field undefined.

- [ ] **Step 3: Add the interface, RouteDeps field, and handler**

In `http.go`:

```go
// InstallNotifier delivers an install_skill_request to a connected agent over
// whatever transport cc-control already uses (chat WS in v1). Defined as an
// interface so the registry package stays independent of the WS layer.
type InstallNotifier interface {
	NotifyInstall(targetAgentID string, sk StoredSkill) error
}

// Add to RouteDeps:
//   Installer InstallNotifier
```

Then the handler:

```go
func (d *RouteDeps) handleInstallRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	actor, err := d.Identity.ResolveActor(r)
	if err != nil {
		writeJSON(w, 401, errBody{Code: "unauth"})
		return
	}
	if actor.Kind != "operator" {
		writeJSON(w, 403, errBody{Code: "forbidden", Reason: "operators only"})
		return
	}
	var req struct {
		Name          string `json:"name"`
		Version       int    `json:"version"` // 0 = latest
		TargetAgentID string `json:"target_agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody{Code: "bad_json", Reason: err.Error()})
		return
	}
	if req.Name == "" || req.TargetAgentID == "" {
		writeJSON(w, 400, errBody{Code: "missing_field"})
		return
	}
	got, err := d.Store.Get(req.Name, req.Version)
	if err == ErrNotFound {
		writeJSON(w, 404, errBody{Code: "not_found"})
		return
	}
	if err != nil {
		writeJSON(w, 500, errBody{Code: "store", Reason: err.Error()})
		return
	}
	if d.Installer == nil {
		writeJSON(w, 503, errBody{Code: "no_installer"})
		return
	}
	if err := d.Installer.NotifyInstall(req.TargetAgentID, *got); err != nil {
		writeJSON(w, 502, errBody{Code: "notify_failed", Reason: err.Error()})
		return
	}
	w.WriteHeader(202)
}
```

- [ ] **Step 4: Run tests (expect pass)**

Run: `cd cc-control && go test ./internal/registry/ -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add cc-control/internal/registry/http.go cc-control/internal/registry/http_test.go
git commit -m "feat(registry): POST /api/registry/install_request with notifier hook"
```

---

### Task 10: Wire registry into cc-control HTTP server + main

**Files:**
- Modify: `cc-control/internal/http/server.go`
- Modify: `cc-control/cmd/cc-control/main.go`

- [ ] **Step 1: Read current server.go to find the right insertion point**

Run: `grep -n "Router\|mux.Handle\|Server struct" cc-control/internal/http/server.go | head -10`
Note the `Server` struct location and the `mux := http.NewServeMux()` site in `Router()`.

- [ ] **Step 2: Extend the Server struct with a registry field**

In `cc-control/internal/http/server.go`, add to the `Server` struct (keep existing fields):

```go
import (
	// ... existing imports
	"cc-control/internal/registry"
)

type Server struct {
	CP          *core.ControlPlane
	Tokens      *auth.Store
	UIDir       string
	CheckOrigin bool
	Registry    *registry.RouteDeps // optional; nil = registry disabled
}
```

- [ ] **Step 3: Wire registry routes inside Router()**

After existing route registrations:

```go
if s.Registry != nil {
	registry.RegisterRoutes(mux, s.Registry)
}
```

- [ ] **Step 4: Implement the InstallNotifier on top of the existing chat WS**

Wherever cc-control currently sends to a connected agent (look in `internal/core/control_plane.go` and `internal/ws/agent.go` for the existing send-to-agent helper — search for `SendToAgent`, `BroadcastTo`, or `agent.send <-`), add a small adapter:

```go
// In a new file, e.g. cc-control/internal/registry/cp_notifier.go:

package registry

import (
	"encoding/json"

	"cc-control/internal/core"
)

// CPNotifier wraps the existing control plane to deliver install_skill_request
// envelopes to a specific agent.
type CPNotifier struct {
	CP *core.ControlPlane
}

func (n *CPNotifier) NotifyInstall(targetAgentID string, sk StoredSkill) error {
	body, _ := json.Marshal(sk)
	env := core.Envelope{
		Type: "install_skill_request",
		Body: body,
	}
	return n.CP.SendToAgent(targetAgentID, env)
}
```

> **Adjust to actual API:** if `core.Envelope`/`SendToAgent` have different names or signatures, match them. The point is: cc-control already has *some* mechanism to route a JSON message to a named agent. Use it.

- [ ] **Step 5: Open registry DB in main.go**

In `cc-control/cmd/cc-control/main.go`:

```go
import (
	// ... existing
	"cc-agent/internal/tools" // for the canonical KnownTools list — or duplicate the slice
	"cc-control/internal/registry"
)

// In main(), after building Server but before .Router():
var regDeps *registry.RouteDeps
if cfg.RegistryDBPath != "" {
	store, err := registry.OpenStore(cfg.RegistryDBPath)
	if err != nil {
		log.Fatalf("open registry: %v", err)
	}
	defer store.Close()
	regDeps = &registry.RouteDeps{
		Store:      store,
		KnownTools: []string{"bash", "read", "write", "grep", "glob", "sysinfo", "proclist", "logtail"},
		Identity:   &registry.IdentityResolver{Auth: tokens}, // tokens is the existing *auth.Store
		Installer:  &registry.CPNotifier{CP: cp},
	}
}
srv := &httpapi.Server{ /* existing fields */, Registry: regDeps}
```

Add `RegistryDBPath` to whatever config struct cc-control reads. Look at where the existing sqlite path is read; add the new flag/field/env in the same place.

- [ ] **Step 6: Build everything**

Run: `go build ./...` from the workspace root.
Expected: clean build for all three modules.

- [ ] **Step 7: Smoke-test the wiring manually**

Run cc-control with a registry DB path set, then `curl -sH 'Authorization: Bearer <a-valid-token>' http://localhost:18180/api/registry/skills`. Expected: `[]`.

- [ ] **Step 8: Commit**

```bash
git add cc-control/internal/http/server.go cc-control/cmd/cc-control/main.go cc-control/internal/registry/cp_notifier.go
git commit -m "feat(registry): wire routes + sqlite + CP notifier into cc-control"
```

---

## Phase C — cc-agent registry client

### Task 11: RegistryClient.Publish

**Files:**
- Create: `cc-agent/internal/skills/client.go`
- Test: `cc-agent/internal/skills/client_test.go`

- [ ] **Step 1: Write failing test**

```go
package skills

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_Publish_RoundTrip(t *testing.T) {
	got := struct {
		path   string
		method string
		auth   string
		body   Skill
	}{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path = r.URL.Path
		got.method = r.Method
		got.auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got.body)
		_, _ = io.WriteString(w, `{"name":"x","version":7}`)
	}))
	defer srv.Close()
	c := &RegistryClient{BaseURL: srv.URL, AgentToken: "tok", HTTP: srv.Client()}
	v, err := c.Publish(&Skill{Name: "x", Prompt: "p"})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if v != 7 {
		t.Fatalf("got %d, want 7", v)
	}
	if got.path != "/api/registry/skills" || got.method != "POST" || !strings.HasSuffix(got.auth, " tok") {
		t.Fatalf("request: %+v", got)
	}
	if got.body.Name != "x" {
		t.Fatalf("body: %+v", got.body)
	}
}

func TestClient_Publish_PropagatesValidationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(400)
		_, _ = io.WriteString(w, `{"code":"invalid_skill","field":"name","reason":"bad"}`)
	}))
	defer srv.Close()
	c := &RegistryClient{BaseURL: srv.URL, AgentToken: "tok", HTTP: srv.Client()}
	_, err := c.Publish(&Skill{Name: "BAD", Prompt: "p"})
	if err == nil {
		t.Fatalf("want error")
	}
	if !strings.Contains(err.Error(), "invalid_skill") {
		t.Fatalf("err = %v", err)
	}
}
```

- [ ] **Step 2: Run tests (expect failure)**

Run: `cd cc-agent && go test ./internal/skills/ -run Publish -v`
Expected: FAIL — `RegistryClient` undefined.

- [ ] **Step 3: Implement client.go (Publish only)**

```go
package skills

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// RegistryClient calls cc-control's /api/registry/* endpoints.
type RegistryClient struct {
	BaseURL    string
	AgentToken string
	HTTP       *http.Client
}

func (c *RegistryClient) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c *RegistryClient) do(method, path string, body any) (*http.Response, error) {
	var buf *bytes.Buffer
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal: %w", err)
		}
		buf = bytes.NewBuffer(raw)
	}
	var bodyReader *bytes.Reader
	_ = bodyReader
	url := c.BaseURL + path
	var req *http.Request
	var err error
	if buf == nil {
		req, err = http.NewRequest(method, url, nil)
	} else {
		req, err = http.NewRequest(method, url, buf)
		req.Header.Set("Content-Type", "application/json")
	}
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.AgentToken)
	return c.httpClient().Do(req)
}

func (c *RegistryClient) Publish(s *Skill) (version int, err error) {
	resp, err := c.do("POST", "/api/registry/skills", s)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, decodeAPIError(resp)
	}
	var out struct {
		Version int `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, fmt.Errorf("decode: %w", err)
	}
	return out.Version, nil
}

func decodeAPIError(resp *http.Response) error {
	var e struct {
		Code   string `json:"code"`
		Field  string `json:"field"`
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&e)
	return fmt.Errorf("registry error: %s (%s) %s", e.Code, e.Field, e.Reason)
}
```

- [ ] **Step 4: Run tests (expect pass)**

Run: `cd cc-agent && go test ./internal/skills/ -run Publish -v`
Expected: both PASS.

- [ ] **Step 5: Commit**

```bash
git add cc-agent/internal/skills/client.go cc-agent/internal/skills/client_test.go
git commit -m "feat(cc-agent): RegistryClient.Publish"
```

---

### Task 12: Client.Install — atomic write to team/

**Files:**
- Modify: `cc-agent/internal/skills/client.go`
- Modify: `cc-agent/internal/skills/client_test.go`

- [ ] **Step 1: Append failing tests**

```go
import (
	"os"
	"path/filepath"
)

func TestClient_Install_AtomicWriteAndReload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"name":"nginx-triage","version":3,"prompt":"p","tools":["bash"],"author_server_id":"ops-01","created_at_unix":100}`)
	}))
	defer srv.Close()
	dir := t.TempDir()
	teamDir := filepath.Join(dir, "team")
	c := &RegistryClient{BaseURL: srv.URL, AgentToken: "t", HTTP: srv.Client(), TeamDir: teamDir}
	got, err := c.Install("nginx-triage", 0)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got.Version != 3 {
		t.Fatalf("version=%d", got.Version)
	}
	written, err := os.ReadFile(filepath.Join(teamDir, "nginx-triage.json"))
	if err != nil {
		t.Fatalf("read installed: %v", err)
	}
	if !strings.Contains(string(written), `"nginx-triage"`) {
		t.Fatalf("body: %s", written)
	}
	// Repeated install must succeed (idempotent).
	if _, err := c.Install("nginx-triage", 0); err != nil {
		t.Fatalf("second Install: %v", err)
	}
}

func TestClient_Install_VersionedURL(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		_, _ = io.WriteString(w, `{"name":"x","version":2,"prompt":"p","author_server_id":"o","created_at_unix":1}`)
	}))
	defer srv.Close()
	dir := t.TempDir()
	c := &RegistryClient{BaseURL: srv.URL, AgentToken: "t", HTTP: srv.Client(), TeamDir: dir}
	if _, err := c.Install("x", 2); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotURL, "version=2") {
		t.Fatalf("url=%s", gotURL)
	}
}
```

- [ ] **Step 2: Run tests (expect failure)**

Run: `cd cc-agent && go test ./internal/skills/ -run Install -v`
Expected: FAIL — `Install`, `TeamDir` undefined.

- [ ] **Step 3: Add Install + StoredSkill type**

In `client.go`, add:

```go
import (
	"os"
	"path/filepath"
	"net/url"
)

type StoredSkill struct {
	Skill
	Version          int    `json:"version"`
	AuthorServerID   string `json:"author_server_id"`
	CreatedAtUnix    int64  `json:"created_at_unix"`
}

// Add to RegistryClient struct:
//   TeamDir string  // where installed skills live (e.g. <skills_dir>/team)

func (c *RegistryClient) Install(name string, version int) (*StoredSkill, error) {
	q := ""
	if version > 0 {
		q = "?" + url.Values{"version": []string{fmt.Sprint(version)}}.Encode()
	}
	resp, err := c.do("GET", "/api/registry/skills/"+name+q, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, decodeAPIError(resp)
	}
	var got StoredSkill
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if c.TeamDir == "" {
		return &got, nil // tests that only assert HTTP roundtrip
	}
	if err := os.MkdirAll(c.TeamDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir team: %w", err)
	}
	final := filepath.Join(c.TeamDir, got.Name+".json")
	tmp := final + ".tmp"
	body, _ := json.MarshalIndent(got.Skill, "", "  ")
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return nil, fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return nil, fmt.Errorf("rename: %w", err)
	}
	return &got, nil
}
```

- [ ] **Step 4: Run tests (expect pass)**

Run: `cd cc-agent && go test ./internal/skills/ -run Install -v`
Expected: both PASS.

- [ ] **Step 5: Commit**

```bash
git add cc-agent/internal/skills/client.go cc-agent/internal/skills/client_test.go
git commit -m "feat(cc-agent): RegistryClient.Install with atomic write to team/"
```

---

### Task 13: Client.List, Client.Get, Client.History

**Files:**
- Modify: `cc-agent/internal/skills/client.go`
- Modify: `cc-agent/internal/skills/client_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestClient_List(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[{"name":"x","description":"demo","version":3,"author_server_id":"a","created_at_unix":1}]`)
	}))
	defer srv.Close()
	c := &RegistryClient{BaseURL: srv.URL, AgentToken: "t", HTTP: srv.Client()}
	got, err := c.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "x" {
		t.Fatalf("got %+v", got)
	}
}

func TestClient_List_PassesQuery(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		_, _ = io.WriteString(w, `[]`)
	}))
	defer srv.Close()
	c := &RegistryClient{BaseURL: srv.URL, AgentToken: "t", HTTP: srv.Client()}
	_, _ = c.List("nginx")
	if !strings.Contains(gotURL, "q=nginx") {
		t.Fatalf("url=%s", gotURL)
	}
}

func TestClient_History(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/history") {
			t.Errorf("path=%s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `[{"name":"x","version":1,"author_server_id":"a","created_at_unix":1}]`)
	}))
	defer srv.Close()
	c := &RegistryClient{BaseURL: srv.URL, AgentToken: "t", HTTP: srv.Client()}
	hist, err := c.History("x")
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 1 {
		t.Fatalf("hist=%+v", hist)
	}
}
```

- [ ] **Step 2: Run tests (expect failure)**

Run: `cd cc-agent && go test ./internal/skills/ -run "List|History" -v`

- [ ] **Step 3: Add the methods + Summary type**

In `client.go`:

```go
type Summary struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	Version          int    `json:"version"`
	AuthorServerID   string `json:"author_server_id"`
	CreatedAtUnix    int64  `json:"created_at_unix"`
}

func (c *RegistryClient) List(query string) ([]Summary, error) {
	path := "/api/registry/skills"
	if query != "" {
		path += "?" + url.Values{"q": []string{query}}.Encode()
	}
	resp, err := c.do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, decodeAPIError(resp)
	}
	var out []Summary
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *RegistryClient) History(name string) ([]Summary, error) {
	resp, err := c.do("GET", "/api/registry/skills/"+name+"/history", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, decodeAPIError(resp)
	}
	var out []Summary
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests (expect pass)**

Run: `cd cc-agent && go test ./internal/skills/ -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add cc-agent/internal/skills/client.go cc-agent/internal/skills/client_test.go
git commit -m "feat(cc-agent): RegistryClient.List + History"
```

---

### Task 14: Two-phase loader (team wins over local)

**Files:**
- Create: `cc-agent/internal/skills/load.go`
- Test: `cc-agent/internal/skills/load_test.go`

- [ ] **Step 1: Write failing test**

```go
package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTwoPhase_TeamOverridesLocal(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, "local")
	team := filepath.Join(root, "team")
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(team, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "x.json"), []byte(`{"name":"x","prompt":"local"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(team, "x.json"), []byte(`{"name":"x","prompt":"team"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry()
	if err := LoadTwoPhase(r, local, team); err != nil {
		t.Fatalf("LoadTwoPhase: %v", err)
	}
	got, ok := r.Get("x")
	if !ok {
		t.Fatalf("not loaded")
	}
	if got.Prompt != "team" {
		t.Fatalf("Prompt=%q, want team", got.Prompt)
	}
}

func TestLoadTwoPhase_LocalOnlyWhenTeamMissing(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, "local")
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "x.json"), []byte(`{"name":"x","prompt":"local"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry()
	if err := LoadTwoPhase(r, local, filepath.Join(root, "team")); err != nil {
		t.Fatalf("LoadTwoPhase: %v", err)
	}
	got, _ := r.Get("x")
	if got.Prompt != "local" {
		t.Fatalf("Prompt=%q, want local", got.Prompt)
	}
}
```

- [ ] **Step 2: Run tests (expect failure)**

Run: `cd cc-agent && go test ./internal/skills/ -run LoadTwoPhase -v`
Expected: FAIL — `LoadTwoPhase` undefined.

- [ ] **Step 3: Implement load.go**

```go
package skills

// LoadTwoPhase loads localDir first, then teamDir, into r. Because the
// underlying registry's map insert is last-write-wins, team skills override
// local ones with the same name — deterministically, regardless of the
// underlying filesystem walk order.
//
// Either directory may be missing; missing directories are ignored.
func LoadTwoPhase(r *Registry, localDir, teamDir string) error {
	if err := r.LoadDir(localDir); err != nil {
		return err
	}
	return r.LoadDir(teamDir)
}
```

- [ ] **Step 4: Run tests (expect pass)**

Run: `cd cc-agent && go test ./internal/skills/ -run LoadTwoPhase -v`
Expected: both PASS.

- [ ] **Step 5: Commit**

```bash
git add cc-agent/internal/skills/load.go cc-agent/internal/skills/load_test.go
git commit -m "feat(cc-agent): two-phase loader so team/ overrides local/"
```

---

## Phase D — cc-agent REPL slash commands

### Task 15: Config — ControlHTTPURL field + plumbing

**Files:**
- Modify: `cc-agent/internal/config/config.go`
- Modify: `cc-agent/internal/config/config_test.go`
- Modify: `cc-agent/cmd/cc-agent/main.go`

- [ ] **Step 1: Add field to Config struct + env handling**

In `config.go`, add to `Config`:

```go
ControlHTTPURL string `json:"control_http_url"`
```

And in `applyEnv()`:

```go
if v := os.Getenv("CC_AGENT_CONTROL_HTTP_URL"); v != "" {
	c.ControlHTTPURL = v
}
```

In `applyDefaults()` (or wherever defaults are applied), derive from `ControlURL`:

```go
if c.ControlHTTPURL == "" && c.ControlURL != "" {
	// ws://host:port → http://host:port; wss → https
	switch {
	case strings.HasPrefix(c.ControlURL, "wss://"):
		c.ControlHTTPURL = "https://" + strings.TrimPrefix(c.ControlURL, "wss://")
	case strings.HasPrefix(c.ControlURL, "ws://"):
		c.ControlHTTPURL = "http://" + strings.TrimPrefix(c.ControlURL, "ws://")
	}
	// strip the WS path suffix if cc-control's WS is at /ws/agent
	if i := strings.Index(c.ControlHTTPURL[len("http://"):], "/"); i >= 0 {
		c.ControlHTTPURL = c.ControlHTTPURL[:len("http://")+i]
	}
}
```

(Add `import "strings"` if not already present.)

- [ ] **Step 2: Add a config test**

```go
func TestConfig_ControlHTTPURL_DerivedFromWS(t *testing.T) {
	cases := map[string]string{
		"ws://example.com:18180/ws/agent": "http://example.com:18180",
		"wss://example.com/ws/agent":      "https://example.com",
		"":                                 "",
	}
	for in, want := range cases {
		c := &Config{ControlURL: in}
		c.applyDefaults()
		if c.ControlHTTPURL != want {
			t.Errorf("ControlURL=%q → ControlHTTPURL=%q, want %q", in, c.ControlHTTPURL, want)
		}
	}
}
```

- [ ] **Step 3: Run config tests**

Run: `cd cc-agent && go test ./internal/config/ -v`
Expected: PASS.

- [ ] **Step 4: Build a RegistryClient at startup**

In `cc-agent/cmd/cc-agent/main.go`, after building the agent and before starting the REPL:

```go
import "cc-agent/internal/skills"

var registryClient *skills.RegistryClient
if cfg.ControlHTTPURL != "" && cfg.AgentToken != "" {
	registryClient = &skills.RegistryClient{
		BaseURL:    cfg.ControlHTTPURL,
		AgentToken: cfg.AgentToken,
		TeamDir:    filepath.Join(cfg.SkillsDir, "team"),
	}
}
```

Use `skills.LoadTwoPhase` for skills loading:

```go
skillsReg := skills.NewRegistry()
if cfg.SkillsDir != "" {
	if err := skills.LoadTwoPhase(skillsReg, filepath.Join(cfg.SkillsDir, "local"), filepath.Join(cfg.SkillsDir, "team")); err != nil {
		// fallback: try the flat dir for backwards compat
		if err := skillsReg.LoadDir(cfg.SkillsDir); err != nil {
			log.Fatalf("load skills: %v", err)
		}
	}
}
```

Pass `registryClient` to whatever owns the REPL slash dispatch (currently `transport/cli.go::handleSlashCommand`). Easiest: pack it into a struct that the REPL goroutine receives.

- [ ] **Step 5: Build everything**

Run: `cd cc-agent && go build ./...`
Expected: clean build.

- [ ] **Step 6: Commit**

```bash
git add cc-agent/internal/config/config.go cc-agent/internal/config/config_test.go cc-agent/cmd/cc-agent/main.go
git commit -m "feat(cc-agent): plumb ControlHTTPURL config + RegistryClient at startup"
```

---

### Task 16: REPL `:registry`

**Files:**
- Modify: `cc-agent/internal/transport/cli.go`

- [ ] **Step 1: Find the slash dispatch site**

The current dispatch is in `cc-agent/internal/transport/cli.go:90-143` (`handleSlashCommand`). Extend the function signature to accept a `*skills.RegistryClient`:

```go
func handleSlashCommand(ctx context.Context, ag *agent.Agent, rc *skills.RegistryClient, sessionID, line string) error {
```

Update the caller to pass `rc` (which may be nil — handlers must check).

- [ ] **Step 2: Add the `:registry` case (manual-test, no automated test)**

The REPL is interactive; we cover it with the e2e test in Task 25. Inline addition to the switch:

```go
case ":registry":
	if rc == nil {
		fmt.Println("(registry not configured: set control_http_url + agent_token)")
		return nil
	}
	q := ""
	if len(parts) > 1 {
		q = strings.Join(parts[1:], " ")
	}
	rows, err := rc.List(q)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		fmt.Println("(no skills in registry)")
		return nil
	}
	for _, s := range rows {
		fmt.Printf("  %-30s v%-3d %-12s %s\n", s.Name, s.Version, s.AuthorServerID, s.Description)
	}
	return nil
```

Update the `:help` listing to include the new commands.

- [ ] **Step 3: Build**

Run: `cd cc-agent && go build ./...`
Expected: clean.

- [ ] **Step 4: Manual smoke test against a running cc-control**

Start cc-control with registry enabled, start cc-agent with `control_http_url` set, then in the REPL: `:registry` → expect `(no skills in registry)`.

- [ ] **Step 5: Commit**

```bash
git add cc-agent/internal/transport/cli.go
git commit -m "feat(cc-agent): :registry slash command"
```

---

### Task 17: REPL `:publish`

**Files:**
- Modify: `cc-agent/internal/transport/cli.go`

- [ ] **Step 1: Add `:publish` case**

After `:registry`, add to the switch:

```go
case ":publish":
	if rc == nil {
		fmt.Println("(registry not configured)")
		return nil
	}
	if len(parts) < 2 {
		return fmt.Errorf("usage: :publish <name>")
	}
	name := parts[1]
	reg := ag.Skills()
	if reg == nil {
		return fmt.Errorf("no local skills registry")
	}
	sk, ok := reg.Get(name)
	if !ok {
		return fmt.Errorf("no local skill %q (try :skills to list)", name)
	}
	wire := &skills.Skill{
		Name:        sk.Name,
		Description: sk.Description,
		Prompt:      sk.Prompt,
		Tools:       sk.Tools,
		Examples:    sk.Examples,
	}
	v, err := rc.Publish(wire)
	if err != nil {
		return err
	}
	fmt.Printf("\033[32m✓ published\033[0m %s@%d\n", name, v)
	return nil
```

- [ ] **Step 2: Build**

Run: `cd cc-agent && go build ./...`

- [ ] **Step 3: Manual smoke**

In a running cc-agent: `:reflect demo "demo skill"` then `:publish demo` → expect `✓ published demo@1`. Run `:registry` → see `demo`.

- [ ] **Step 4: Commit**

```bash
git add cc-agent/internal/transport/cli.go
git commit -m "feat(cc-agent): :publish slash command"
```

---

### Task 18: REPL `:install` with preview + y/N

**Files:**
- Modify: `cc-agent/internal/transport/cli.go`

- [ ] **Step 1: Read the existing y/N approver pattern**

Run: `grep -n "y/N\|fmt.Scan\|bufio.NewReader" cc-agent/internal/tools/approval.go cc-agent/internal/transport/cli.go`. Whatever pattern that file uses for the destructive-bash prompt is what we mirror — reuse the same idiom and message style.

- [ ] **Step 2: Add `:install` case**

```go
case ":install":
	if rc == nil {
		fmt.Println("(registry not configured)")
		return nil
	}
	if len(parts) < 2 {
		return fmt.Errorf("usage: :install <name>[@version]")
	}
	name, version := parseNameVersion(parts[1])
	// Fetch without writing first to render the preview.
	preview := *rc            // shallow copy
	preview.TeamDir = ""      // disables file write
	got, err := preview.Install(name, version)
	if err != nil {
		return err
	}
	fmt.Println("\033[36m── skill preview ──\033[0m")
	fmt.Printf("  name:    %s @ v%d\n", got.Name, got.Version)
	fmt.Printf("  author:  %s\n", got.AuthorServerID)
	fmt.Printf("  updated: %s\n", time.Unix(got.CreatedAtUnix, 0).Format(time.RFC3339))
	fmt.Printf("  tools:   %s\n", strings.Join(got.Tools, ", "))
	fmt.Printf("  prompt:  %s\n", clip(got.Prompt, 200))
	if len(got.Examples) > 0 {
		fmt.Println("  examples:")
		for _, e := range got.Examples {
			fmt.Printf("    - %s\n", clip(e, 80))
		}
	}
	fmt.Print("install? [y/N]: ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line != "y" && line != "yes" {
		fmt.Println("aborted.")
		return nil
	}
	if _, err := rc.Install(name, version); err != nil {
		return err
	}
	fmt.Printf("\033[32m✓ installed\033[0m %s@%d → %s/team/%s.json\n", got.Name, got.Version, filepath.Dir(rc.TeamDir), got.Name)
	// Reload skills registry so the new file is picked up.
	if reg := ag.Skills(); reg != nil {
		_ = reg.LoadDir(rc.TeamDir)
	}
	return nil
```

Add the helper at the bottom of the file:

```go
func parseNameVersion(s string) (string, int) {
	if i := strings.LastIndex(s, "@"); i > 0 {
		v, err := strconvAtoi(s[i+1:])
		if err == nil {
			return s[:i], v
		}
	}
	return s, 0
}

func strconvAtoi(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number: %q", s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
```

(Or import `"strconv"` and use `strconv.Atoi`. Match whatever else the file does.)

Imports needed: `"bufio"`, `"os"`, `"time"`, `"path/filepath"`.

- [ ] **Step 3: Build**

Run: `cd cc-agent && go build ./...`

- [ ] **Step 4: Manual smoke**

`:install demo` → see preview, type `y` → file appears in `<skills_dir>/team/demo.json`. Run `:install demo` again → idempotent.

- [ ] **Step 5: Commit**

```bash
git add cc-agent/internal/transport/cli.go
git commit -m "feat(cc-agent): :install with preview + y/N confirm"
```

---

### Task 19: REPL `:history` and `:rollback`

**Files:**
- Modify: `cc-agent/internal/transport/cli.go`

- [ ] **Step 1: Add cases**

```go
case ":history":
	if rc == nil {
		fmt.Println("(registry not configured)")
		return nil
	}
	if len(parts) < 2 {
		return fmt.Errorf("usage: :history <name>")
	}
	hist, err := rc.History(parts[1])
	if err != nil {
		return err
	}
	if len(hist) == 0 {
		fmt.Println("(no history)")
		return nil
	}
	for _, h := range hist {
		fmt.Printf("  v%-3d %-12s %s  %s\n", h.Version, h.AuthorServerID,
			time.Unix(h.CreatedAtUnix, 0).Format(time.RFC3339), h.Description)
	}
	return nil

case ":rollback":
	if len(parts) < 3 {
		return fmt.Errorf("usage: :rollback <name> <version>")
	}
	v, err := strconvAtoi(parts[2])
	if err != nil {
		return err
	}
	// Same path as :install with explicit version. Skip preview prompt for
	// rollback — operator typed the exact version, intent is clear.
	if rc == nil {
		fmt.Println("(registry not configured)")
		return nil
	}
	if _, err := rc.Install(parts[1], v); err != nil {
		return err
	}
	fmt.Printf("\033[32m✓ rolled back\033[0m %s to v%d\n", parts[1], v)
	if reg := ag.Skills(); reg != nil {
		_ = reg.LoadDir(rc.TeamDir)
	}
	return nil
```

Update `:help`:

```go
case ":help", ":?":
	fmt.Println(`commands:
  :help                            show this help
  :skills                          list loaded skills
  :reflect <name> [description]    distill current session into a skill
  :registry [search]               list team skills
  :publish <name>                  push a local skill to the team registry
  :install <name>[@version]        fetch + install a team skill (with preview)
  :history <name>                  show all versions of a team skill
  :rollback <name> <version>       install a specific older version
  exit | quit                      leave`)
```

- [ ] **Step 2: Build**

Run: `cd cc-agent && go build ./...`

- [ ] **Step 3: Manual smoke**

After publishing twice, `:history demo` should show two rows; `:rollback demo 1` should write v1 to disk.

- [ ] **Step 4: Commit**

```bash
git add cc-agent/internal/transport/cli.go
git commit -m "feat(cc-agent): :history + :rollback slash commands"
```

---

## Phase E — cc-web Skills tab

### Task 20: skills.html shell

**Files:**
- Create: `cc-web/skills.html`
- Create: `cc-web/js/main-skills.js`

- [ ] **Step 1: Mirror admin.html's pattern**

Read `cc-web/admin.html` for the page chrome (header, page-body, page-header). Then:

```html
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Claude Code Control Plane - Skills</title>
  <link rel="stylesheet" href="/style.css">
</head>
<body>
  <header>
    <div class="header-title-row">
      <h1>Claude Code Control Plane</h1>
    </div>
  </header>
  <main class="page-body skills-page">
    <div class="page-header">
      <h2>Skills</h2>
      <p class="page-subtitle">Browse and install team skills.</p>
    </div>
    <div class="skills-layout">
      <section class="skills-list-pane">
        <input id="skills-search" class="skills-search" placeholder="Search…" />
        <ul id="skills-list" class="skills-list"></ul>
      </section>
      <section id="skills-detail" class="skills-detail">
        <p class="placeholder">Select a skill on the left.</p>
      </section>
    </div>
  </main>
  <script type="module" src="/js/main-skills.js"></script>
</body>
</html>
```

- [ ] **Step 2: main-skills.js entry**

```js
import { initSkillsPage } from "./skills/page.js";

initSkillsPage();
```

- [ ] **Step 3: Add minimal CSS to cc-web/style.css**

Append to `cc-web/style.css`:

```css
.skills-layout { display: grid; grid-template-columns: 320px 1fr; gap: 16px; }
.skills-list-pane { border-right: 1px solid #333; padding-right: 12px; }
.skills-search { width: 100%; padding: 6px 8px; margin-bottom: 8px; }
.skills-list { list-style: none; padding: 0; margin: 0; }
.skills-list li { padding: 8px; border: 1px solid #333; border-radius: 4px; margin-bottom: 6px; cursor: pointer; }
.skills-list li.selected { border-color: #6cf; background: #0a1a22; }
.skills-detail .placeholder { color: #888; }
@media (max-width: 720px) { .skills-layout { grid-template-columns: 1fr; } }
```

- [ ] **Step 4: Verify the page loads**

(No test — covered by Playwright in Task 25. Build is JS, no compile step.)

- [ ] **Step 5: Commit**

```bash
git add cc-web/skills.html cc-web/js/main-skills.js cc-web/style.css
git commit -m "feat(cc-web): skills.html shell + master/detail layout css"
```

---

### Task 21: js/skills/api.js

**Files:**
- Create: `cc-web/js/skills/api.js`

- [ ] **Step 1: Read the existing http helper**

Run: `grep -n "createAdminApi\|createMessageBinder" cc-web/js/shared/http.js` — see how admin token + fetch is wrapped. We'll do the same shape.

- [ ] **Step 2: Implement api.js**

```js
import { authHeader } from "../shared/http.js"; // adapt to actual export

const BASE = "/api/registry";

async function jsonFetch(method, path, body) {
  const res = await fetch(BASE + path, {
    method,
    headers: { ...authHeader(), "Content-Type": "application/json" },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ code: res.status }));
    throw Object.assign(new Error(err.code || res.statusText), err);
  }
  if (res.status === 204) return null;
  return res.json();
}

export const skillsApi = {
  list:   (q) => jsonFetch("GET", "/skills" + (q ? `?q=${encodeURIComponent(q)}` : "")),
  get:    (name, v) => jsonFetch("GET", `/skills/${name}` + (v ? `?version=${v}` : "")),
  history:(name)  => jsonFetch("GET", `/skills/${name}/history`),
  install:(name, v, target) => jsonFetch("POST", "/install_request", {
    name, version: v || 0, target_agent_id: target,
  }),
};
```

> If `authHeader` doesn't exist as an export, look at how admin/page.js attaches the token to requests and inline that shape here. The token comes from the existing operator session.

- [ ] **Step 3: Commit**

```bash
git add cc-web/js/skills/api.js
git commit -m "feat(cc-web): skills API client"
```

---

### Task 22: js/skills/render.js

**Files:**
- Create: `cc-web/js/skills/render.js`

- [ ] **Step 1: Pure rendering helpers**

```js
export function renderList(items, container, onSelect) {
  container.innerHTML = "";
  if (!items || items.length === 0) {
    container.innerHTML = `<li class="placeholder">No skills.</li>`;
    return;
  }
  for (const item of items) {
    const li = document.createElement("li");
    li.dataset.name = item.name;
    li.innerHTML = `
      <div class="skills-row-name">${escapeHtml(item.name)}</div>
      <div class="skills-row-meta">v${item.version} · ${escapeHtml(item.author_server_id)} · ${formatAge(item.created_at_unix)}</div>
      <div class="skills-row-desc">${escapeHtml(item.description || "")}</div>
    `;
    li.addEventListener("click", () => onSelect(item.name));
    container.appendChild(li);
  }
}

export function renderDetail(skill, history, container, onInstall) {
  if (!skill) {
    container.innerHTML = `<p class="placeholder">Select a skill on the left.</p>`;
    return;
  }
  container.innerHTML = `
    <div class="skills-detail-head">
      <h3>${escapeHtml(skill.name)} <small>v${skill.version}</small></h3>
      <div class="muted">by ${escapeHtml(skill.author_server_id)} · ${formatAge(skill.created_at_unix)}</div>
    </div>
    <h4>Prompt</h4>
    <pre class="skills-prompt">${escapeHtml(skill.prompt)}</pre>
    <h4>Tools</h4>
    <div class="skills-tools">${(skill.tools || []).map(t => `<span class="tag">${escapeHtml(t)}</span>`).join("")}</div>
    ${(skill.examples && skill.examples.length) ? `
      <h4>Examples</h4>
      <ul>${skill.examples.map(e => `<li>${escapeHtml(e)}</li>`).join("")}</ul>` : ""}
    <h4>History</h4>
    <table class="skills-history">
      <tr><th>Version</th><th>Author</th><th>When</th><th>Description</th></tr>
      ${(history || []).map(h => `
        <tr><td>v${h.version}</td><td>${escapeHtml(h.author_server_id)}</td>
            <td>${formatAge(h.created_at_unix)}</td><td>${escapeHtml(h.description || "")}</td></tr>
      `).join("")}
    </table>
    <button id="skills-install-btn" class="btn-primary">Install on host…</button>
  `;
  container.querySelector("#skills-install-btn").addEventListener("click", () => onInstall(skill));
}

function escapeHtml(s) {
  return String(s ?? "").replace(/[&<>"']/g, c => ({"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;","'":"&#39;"}[c]));
}

function formatAge(unix) {
  if (!unix) return "";
  const sec = Math.max(0, Math.floor(Date.now() / 1000) - unix);
  if (sec < 60) return `${sec}s ago`;
  if (sec < 3600) return `${Math.floor(sec / 60)}m ago`;
  if (sec < 86400) return `${Math.floor(sec / 3600)}h ago`;
  return `${Math.floor(sec / 86400)}d ago`;
}
```

Append to `cc-web/style.css`:

```css
.skills-row-name { color: #6cf; font-weight: 600; }
.skills-row-meta { color: #888; font-size: 11px; }
.skills-row-desc { font-size: 12px; margin-top: 2px; }
.skills-prompt { background: #111; padding: 8px; border-radius: 4px; white-space: pre-wrap; }
.skills-tools .tag { display: inline-block; background: #222; padding: 2px 6px; border-radius: 3px; margin-right: 4px; font-size: 11px; }
.skills-history { width: 100%; border-collapse: collapse; }
.skills-history th, .skills-history td { padding: 4px 8px; border-bottom: 1px solid #222; text-align: left; font-size: 12px; }
.muted { color: #888; font-size: 12px; }
```

- [ ] **Step 2: Commit**

```bash
git add cc-web/js/skills/render.js cc-web/style.css
git commit -m "feat(cc-web): list + detail renderers"
```

---

### Task 23: js/skills/page.js — state machine

**Files:**
- Create: `cc-web/js/skills/page.js`

- [ ] **Step 1: Wire interactions**

```js
import { skillsApi } from "./api.js";
import { renderList, renderDetail } from "./render.js";

export function initSkillsPage() {
  const listEl = document.getElementById("skills-list");
  const detailEl = document.getElementById("skills-detail");
  const searchEl = document.getElementById("skills-search");

  const state = { items: [], selected: null, history: [], detail: null, query: "" };

  async function reloadList() {
    state.items = await skillsApi.list(state.query);
    renderList(state.items, listEl, onSelect);
    if (state.selected && !state.items.find(i => i.name === state.selected)) {
      state.selected = null;
      state.detail = null;
      renderDetail(null, [], detailEl, onInstall);
    } else if (!state.selected && state.items.length > 0) {
      onSelect(state.items[0].name);
    }
  }

  async function onSelect(name) {
    state.selected = name;
    listEl.querySelectorAll("li").forEach(li => li.classList.toggle("selected", li.dataset.name === name));
    const [detail, history] = await Promise.all([
      skillsApi.get(name),
      skillsApi.history(name),
    ]);
    state.detail = detail;
    state.history = history;
    renderDetail(detail, history, detailEl, onInstall);
  }

  async function onInstall(skill) {
    const target = window.prompt("Install on which host? (server_id)");
    if (!target) return;
    try {
      await skillsApi.install(skill.name, skill.version, target);
      alert(`install request sent to ${target}`);
    } catch (err) {
      alert(`install failed: ${err.code || err.message}`);
    }
  }

  searchEl.addEventListener("input", debounce(() => {
    state.query = searchEl.value.trim();
    reloadList();
  }, 200));

  reloadList().catch(err => {
    detailEl.innerHTML = `<p class="placeholder">Failed to load: ${err.message}</p>`;
  });
}

function debounce(fn, ms) {
  let t = null;
  return (...args) => {
    clearTimeout(t);
    t = setTimeout(() => fn(...args), ms);
  };
}
```

- [ ] **Step 2: Manual smoke**

Visit `/skills.html` in a running stack. Search filters list. Click a row → detail loads. Click Install → prompt for host, submit → "install request sent to …".

- [ ] **Step 3: Commit**

```bash
git add cc-web/js/skills/page.js
git commit -m "feat(cc-web): skills page state machine + install flow"
```

---

## Phase F — Integration tests

### Task 24: Go integration smoke (`tests/registry_e2e/`)

**Files:**
- Create: `tests/registry_e2e/main_test.go`

- [ ] **Step 1: Write the smoke test**

```go
package registry_e2e

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	httpapi "cc-control/internal/http"
	"cc-control/internal/auth"
	"cc-control/internal/core"
	"cc-control/internal/registry"

	"cc-agent/internal/skills"
)

func TestRegistry_PublishInstallRollback(t *testing.T) {
	dir := t.TempDir()
	store, err := registry.OpenStore(filepath.Join(dir, "registry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	tokens := auth.NewMemoryStore() // adjust to real constructor
	tokA := tokens.IssueAgent("ops-A")
	tokB := tokens.IssueAgent("ops-B")
	deps := &registry.RouteDeps{
		Store:      store,
		KnownTools: []string{"bash"},
		Identity:   &registry.IdentityResolver{Auth: tokens},
		Installer:  nil, // not exercising install_request here
	}
	srv := httptest.NewServer(buildRouter(deps))
	defer srv.Close()

	clientA := &skills.RegistryClient{BaseURL: srv.URL, AgentToken: tokA, TeamDir: filepath.Join(dir, "A", "team")}
	clientB := &skills.RegistryClient{BaseURL: srv.URL, AgentToken: tokB, TeamDir: filepath.Join(dir, "B", "team")}

	// 1. A publishes v1.
	if v, err := clientA.Publish(&skills.Skill{Name: "x", Prompt: "p1", Tools: []string{"bash"}}); err != nil || v != 1 {
		t.Fatalf("A.Publish v1 = %d, %v", v, err)
	}
	// 2. B sees it.
	if rows, err := clientB.List(""); err != nil || len(rows) != 1 || rows[0].Name != "x" {
		t.Fatalf("B.List = %+v, %v", rows, err)
	}
	// 3. B installs.
	if got, err := clientB.Install("x", 0); err != nil || got.Version != 1 {
		t.Fatalf("B.Install = %+v, %v", got, err)
	}
	// 4. A publishes v2.
	if v, err := clientA.Publish(&skills.Skill{Name: "x", Prompt: "p2", Tools: []string{"bash"}}); err != nil || v != 2 {
		t.Fatalf("A.Publish v2 = %d, %v", v, err)
	}
	// 5. B installs latest.
	if got, err := clientB.Install("x", 0); err != nil || got.Version != 2 {
		t.Fatalf("B.Install v2 = %+v, %v", got, err)
	}
	// 6. B rolls back to v1.
	if got, err := clientB.Install("x", 1); err != nil || got.Version != 1 {
		t.Fatalf("B.Install@1 = %+v, %v", got, err)
	}
	// 7. History shows two versions.
	hist, err := clientA.History("x")
	if err != nil || len(hist) != 2 {
		t.Fatalf("A.History = %+v, %v", hist, err)
	}
}

// Helpers — adapt these to whatever the actual cc-control auth.Store and ws
// types expose. The point of this file is to exercise the real registry
// package end-to-end against a real httptest server.
func buildRouter(deps *registry.RouteDeps) http.Handler {
	mux := http.NewServeMux()
	registry.RegisterRoutes(mux, deps)
	return mux
}

var _ = httpapi.Server{} // silence unused-import warning if not all imports land
var _ = core.ControlPlane{}
```

> **Adapter note:** the imports listed above (`auth.NewMemoryStore`, `IssueAgent`) may not exist. Either add the smallest helper to `auth` to mint a token-record-equivalent for tests, or replace `IdentityResolver{Auth: tokens}` with a stub `IdentityProvider` that returns `Actor{Kind:"agent", ID:"ops-A"}` based on the bearer token string. Whatever lets the test compile is fine — the assertions are what matters.

- [ ] **Step 2: Run the test**

Run: `cd tests/registry_e2e && go test -v`

If the test cannot import cc-control or cc-agent because of go.work topology, add a `go.mod` here that uses `replace` directives — or move the test into one of the existing modules. Easier: place it in `cc-control/internal/registry/e2e_test.go` as a same-package test; the cc-agent client side can import cc-agent via the workspace.

- [ ] **Step 3: Iterate until green**

Adjust import paths and adapter helpers until the test compiles and the seven steps all pass.

- [ ] **Step 4: Commit**

```bash
git add tests/registry_e2e/main_test.go
git commit -m "test(registry): integration smoke for publish/install/rollback"
```

---

### Task 25: Playwright UI smoke

**Files:**
- Create: `tests/web-e2e/specs/skills.spec.js`

- [ ] **Step 1: Read existing spec for the harness conventions**

Run: `head -60 tests/web-e2e/specs/notification.spec.js` — note the test fixture, base URL, login flow.

- [ ] **Step 2: Write a minimal spec**

```js
import { test, expect } from "@playwright/test";

test("skills tab lists, selects, and shows install button", async ({ page }) => {
  // Pre-seed the registry by hitting the API directly with a tenant token.
  // Adapt to the harness's existing auth helper.
  await page.request.post("/api/registry/skills", {
    headers: { Authorization: `Bearer ${process.env.CC_E2E_TENANT_TOKEN}` },
    data: { name: "demo-skill", description: "Demo", prompt: "hello", tools: ["bash"] },
  });

  await page.goto("/skills.html");
  await expect(page.getByText("Skills")).toBeVisible();
  await expect(page.locator(".skills-list li").first()).toContainText("demo-skill");

  await page.locator(".skills-list li").first().click();
  await expect(page.locator(".skills-detail h3")).toContainText("demo-skill");
  await expect(page.locator("#skills-install-btn")).toBeVisible();
});
```

- [ ] **Step 3: Run the spec**

Run: `npm run test:web:e2e -- tests/web-e2e/specs/skills.spec.js`

If the harness needs registry to be enabled in cc-control, add `CC_CONTROL_REGISTRY_DB=…` (or whatever the config knob became in Task 10) to `tests/web-e2e/run-harness.sh`.

- [ ] **Step 4: Commit**

```bash
git add tests/web-e2e/specs/skills.spec.js tests/web-e2e/run-harness.sh
git commit -m "test(cc-web): skills tab playwright smoke"
```

---

## Self-review

Spec coverage check (each section/req → task):

- §1 Motivation, §2 Non-goals, §3 Decisions — informational; covered by spec.
- §4 Architecture — Tasks 1, 6–10 (registry package + wiring).
- §5.1 registry package — Tasks 1–10.
- §5.2 cc-agent client + team_dir — Tasks 11–14.
- §5.3 REPL slash commands — Tasks 16–19.
- §5.4 cc-web Skills tab (master/detail) — Tasks 20–23 (note: vanilla JS, not React; documented in plan preamble).
- §5.5 Configuration touch points — Tasks 10, 15.
- §6 Data flow (publish/install/browse/rollback) — Tasks 6, 7, 12, 18, 19, 23.
- §7 Error handling table — covered by Tasks 6, 7, 8, 9 (status codes), 11–13 (client error propagation), 18 (atomic rename).
- §8 Testing — distributed across every TDD task; integration in 24, UI in 25.
- §9 Migration — covered implicitly by Tasks 10 (`registry_db_path` config) and 15 (`control_http_url` config). Backward compatibility enforced: cc-agent without `control_http_url` shows "registry not configured" in `:registry`/`:publish`/etc. (Tasks 16–19).
- §10 Open questions for v2 — explicitly out of scope.

Placeholder scan: I checked for "TBD"/"TODO"/"add appropriate"/"similar to Task" — none. Tasks 5, 10, 24 contain "adjust to actual API" notes; those are intentional adapter callouts (we cannot know the exact `auth.TokenRecord` field names without reading the file at execution time), not placeholders for missing logic.

Type consistency: `Skill` and `StoredSkill` shapes match between `cc-control/internal/registry` and `cc-agent/internal/skills` (both use `name, description, prompt, tools, examples` for body and `version, author_server_id, created_at_unix` for metadata). Method names are consistent: `Publish`, `Install`, `List`, `Get`, `History`, `SoftDelete` everywhere.
