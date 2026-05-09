# cc-agent Skill Marketplace — Design (v1)

**Date:** 2026-05-09
**Status:** Draft, awaiting user review
**Scope:** Team-private skill registry hosted in cc-control, peer push/pull,
explicit install with preview, latest-wins-with-history, primary discovery
via cc-web Skills tab.

## 1. Motivation

cc-agent already turns successful sessions into reusable skills via
`:reflect` (`cc-agent/internal/skills/reflect.go`). Today those skills live
only in `skills_dir/` on the host where they were distilled. A team running
cc-agent across multiple servers has no way to share a skill except `scp` —
which loses authorship, history, and discoverability.

The marketplace is the propagation layer: take the existing skill kernel
format unchanged, add a private hub inside cc-control, and give operators
`:publish` / `:install` plus a browseable Skills tab.

## 2. Non-goals

Explicitly out of scope for v1:

- Public / OSS registry. Trust model assumes one organisation, authenticated
  members only.
- Cryptographic signing of skill bodies. Deferred to v2.
- Subscription / channels / push-on-publish. Install is always operator-driven.
- Cross-org federation, mirroring, content-addressed storage. Deferred to
  v2 if/when public registry lands.
- Skill dependencies (skill A requires skill B). Skills remain self-contained.
- Tool whitelist enforcement at install time against the *target agent's*
  toolset (validation runs against cc-control's known tool list at publish
  time only — see §6).

## 3. Decisions captured during brainstorming

| # | Question                       | Decision                                  |
|---|--------------------------------|--------------------------------------------|
| 1 | Audience                       | Team / private (one org, authed members)   |
| 2 | Hosting                        | Inside cc-control                          |
| 3 | Workflow                       | Peer push + pull                           |
| 4 | Versioning                     | Latest wins, full history kept             |
| 5 | Discovery                      | cc-web Skills tab primary, REPL secondary  |
| 6 | Sync model                     | Explicit install only                      |
| 7 | Install safety                 | Preview + `[y/N]` confirm                  |
| 8 | Architecture                   | A — HTTP-on-cc-control                     |
| 9 | UI layout                      | B — master/detail with side panel          |

## 4. Architecture

```
┌──────────────────┐     HTTP /api/registry      ┌──────────────────────────┐
│   cc-agent (N)   │ ─────────────────────────►  │       cc-control         │
│  REPL :publish   │ ◄─────────────────────────  │  internal/registry/      │
│  REPL :install   │     bearer agent-token      │  ├─ store.go (sqlite)    │
│                  │                             │  ├─ http.go  (handlers)  │
└──────────────────┘                             │  ├─ identity.go          │
                                                 │  └─ validate.go          │
┌──────────────────┐     HTTP /api/registry      │                          │
│   cc-web (UI)    │ ─────────────────────────►  │  internal/cchttp/        │
│  /skills page    │     operator session        │   /api/registry/skills…  │
└──────────────────┘                             └────────────┬─────────────┘
                                                              │
                                                              ▼
                                                 ┌──────────────────────────┐
                                                 │ registry.db (sqlite)     │
                                                 │ skills(name, version,    │
                                                 │   body_json, author,     │
                                                 │   created_at, deleted_at)│
                                                 └──────────────────────────┘
```

Three actors, one REST API, one sqlite store. Both agents and the web UI
call the same endpoints; cc-control owns the data and the auth boundary.
The registry has no dependency on cc-control's existing chat WS bridge — it
works even when an agent isn't currently chat-connected.

Persistence is its own sqlite DB (`registry.db`), separate from the existing
session store, so backups and migrations stay independent.

## 5. Components

### 5.1 `cc-control/internal/registry/` (new package, ~400 LOC)

**`store.go`** — sqlite layer. One table:

```sql
CREATE TABLE skills (
  name           TEXT NOT NULL,
  version        INTEGER NOT NULL,
  body_json      TEXT NOT NULL,
  author_server_id TEXT NOT NULL,
  created_at     INTEGER NOT NULL,    -- unix seconds
  deleted_at     INTEGER,             -- nullable; soft-delete
  PRIMARY KEY (name, version)
);
CREATE INDEX skills_name_idx ON skills(name);
CREATE INDEX skills_author_idx ON skills(author_server_id);
```

Methods (no HTTP, no auth — pure data):

- `Publish(skill *Skill, author string) (version int, err error)` — inside a
  `BEGIN IMMEDIATE` tx, computes `MAX(version)+1` and INSERTs.
- `Get(name string, version int) (*StoredSkill, error)` — returns latest if
  `version == 0`.
- `Latest(name string) (*StoredSkill, error)`.
- `List(query string) ([]SkillSummary, error)` — name/description LIKE search,
  excludes soft-deleted by default.
- `History(name string) ([]SkillSummary, error)` — all versions, oldest first,
  excludes soft-deleted by default.
- `SoftDelete(name string, version int, by string) error` — admin-only;
  enforced by handler, not store.

**`http.go`** — `RegisterRoutes(mux *http.ServeMux, store *Store, identity *IdentityResolver)`. Wires REST handlers, parses JSON, sets `Content-Type: application/json`, returns proper status codes. Auth check happens before any store call.

REST surface:

| Method | Path                                      | Auth required | Notes                                |
|--------|-------------------------------------------|---------------|--------------------------------------|
| POST   | `/api/registry/skills`                    | agent or operator | Body = skill JSON. Returns 201 + version. |
| GET    | `/api/registry/skills`                    | agent or operator | Optional `?q=` filter. Returns latest of each name. |
| GET    | `/api/registry/skills/:name`              | agent or operator | Returns latest. `?version=N` for pin. |
| GET    | `/api/registry/skills/:name/history`      | agent or operator | All versions for that name.          |
| DELETE | `/api/registry/skills/:name/:version`     | operator + admin  | Soft-delete only.                    |

**`identity.go`** — `ResolveActor(r *http.Request) (Actor, error)`:

```go
type Actor struct {
    Kind    string  // "agent" | "operator"
    ID      string  // server_id (agent) or user_id (operator)
    IsAdmin bool    // operator-only flag from existing cc-web session claims
}
```

Reads `Authorization: Bearer <agent-token>` for the agent path or the
existing cc-web session cookie for the operator path. Reuses cc-control's
existing token verification — no new identity infrastructure.

**`validate.go`** — schema sanity on publish:

- `name` must match `^[a-z][a-z0-9-]{1,63}$`
- `prompt` non-empty, ≤ 32 KiB
- `tools` is a subset of cc-control's known tool list (currently
  `bash, read, write, grep, glob, sysinfo, proclist, logtail` from
  `cc-agent/README.md:88-97`)
- `examples` ≤ 10 entries, each ≤ 1 KiB
- Total body ≤ 64 KiB

Rejects pre-store; failures return `400` with the offending field.

### 5.2 `cc-agent/internal/skills/` (extend existing package, ~150 LOC)

**`client.go`** — `RegistryClient`:

```go
type RegistryClient struct {
    BaseURL    string
    AgentToken string
    HTTP       *http.Client
}

func (c *RegistryClient) Publish(s *Skill) (version int, err error)
func (c *RegistryClient) Install(name string, version int) (*Skill, error)
func (c *RegistryClient) List(query string) ([]SkillSummary, error)
func (c *RegistryClient) History(name string) ([]SkillSummary, error)
```

Install writes JSON atomically (`os.Rename` from a temp file in the same
directory) to `skills_dir/team/<name>.json`, then triggers the existing
registry reload.

**`team_dir.go`** — convention:

- `skills_dir/team/<name>.json` — installed from registry.
- `skills_dir/local/<name>.json` — locally authored via `:reflect`.
- On load, both subtrees scan; same-name collision = team wins. Operator
  removes the team file to override locally.

### 5.3 `cc-agent/cmd/cc-agent/repl.go` (extend existing REPL)

New slash commands:

| Command                                     | Action                                                    |
|---------------------------------------------|-----------------------------------------------------------|
| `:publish <name> [--description=…]`         | Push the named local skill to cc-control.                 |
| `:install <name>[@version]`                 | Fetch, preview (prompt + tools + author + last-updated), prompt `[y/N]`, install to `team/`. |
| `:registry [search-term]`                   | List remote skills (name, version, author, updated).      |
| `:history <name>`                           | Show all versions of one skill.                           |
| `:rollback <name> <version>`                | Sugar for `:install name@version`.                        |

`:install` reuses the same confirmation primitive as the destructive-bash
approver in `cc-agent/internal/tools/approval.go` — same `[y/N]` prompt
pattern operators already know.

### 5.4 `cc-web/src/pages/skills/` (new React route, ~300 LOC)

Layout: **master/detail with side panel** (Q9 decision). One screen, no
page navigation between list and detail.

- `SkillsPage.tsx` — split layout. Left: search input + scrollable list of
  skill cards (name, version, author, updated, one-line description).
  Right: side panel for the currently-selected skill.
- `SkillDetail.tsx` — embedded right panel: full prompt (monospace block),
  tool whitelist as tag chips, examples as collapsible list, version history
  table, "Install on host…" button (opens a dropdown of currently-connected
  agents from cc-control's existing fleet view).
- `api.ts` — typed client for `/api/registry/*`.

On viewports below 720 px, the side panel collapses to a full-page detail
view (mobile fallback).

### 5.5 Configuration

**`cc-control` config** — new optional field:

```json
"registry_db_path": "./registry.db"
```

Defaults to `registry.db` next to the existing session DB.

**`cc-agent` config** — new optional field:

```json
"control_http_url": "http://your-control:18180"
```

Defaults to deriving `http://` from `control_url`'s `ws://` (same host,
same port, swap scheme). Explicit override needed only when control plane
splits HTTP and WS endpoints.

Total new surface: 1 sqlite table, 5 REST endpoints, 5 REPL commands, 1
React page, 2 config fields. ~1000 LOC.

## 6. Data flow

### 6.1 Publish

```
cc-agent REPL :publish nginx-triage
  ├─ load skills_dir/local/nginx-triage.json
  ├─ POST /api/registry/skills  (Authorization: Bearer <agent-token>)
  │     body = skill JSON
  │     │
  │     ▼  cc-control
  │   1. identity.ResolveActor → Actor{kind: agent, id: "ops-01"}
  │   2. validate.Schema → ok
  │   3. store.Publish(skill, "ops-01"):
  │        BEGIN IMMEDIATE
  │        SELECT MAX(version) FROM skills WHERE name = 'nginx-triage'
  │        INSERT (name, MAX+1, body_json, 'ops-01', now())
  │        COMMIT
  │   4. 201 { name, version: 3, author: "ops-01", created_at }
  └─ print "✓ published nginx-triage@3"
```

### 6.2 Install

```
cc-agent REPL :install nginx-triage
  ├─ GET /api/registry/skills/nginx-triage      (no version → latest)
  ├─ render preview block:
  │     name:       nginx-triage @ v3
  │     author:     ops-01
  │     updated:    2h ago
  │     prompt:     Focus on access.log/error.log under /var/log/nginx; …
  │     tools:      bash, read, grep, logtail
  │     examples:   "nginx is returning 502 — diagnose"
  ├─ prompt "[y/N]"
  ├─ on y:
  │     write skills_dir/team/.nginx-triage.json.tmp
  │     os.Rename → skills_dir/team/nginx-triage.json   (atomic)
  │     registry.Reload()
  └─ print "✓ installed nginx-triage@3"
```

### 6.3 Browse (web UI)

```
cc-web /skills
  ├─ GET /api/registry/skills?q=nginx
  └─ render list (left); first item auto-selects right panel.

select a skill in left list
  ├─ GET /api/registry/skills/<name>
  ├─ GET /api/registry/skills/<name>/history
  └─ render right panel.

click "Install on host…"
  ├─ dropdown of currently-connected agents (existing fleet view)
  ├─ POST /api/registry/install_request { name, version, target_agent_id }
  └─ cc-control sends `install_skill_request` over the existing chat WS
     to that agent. Agent runs the same install path as REPL but skips
     the y/N — operator already confirmed in UI.
```

### 6.4 History / rollback

```
:history nginx-triage
  └─ GET /api/registry/skills/nginx-triage/history
     → table of (version, author, created_at, summary)

:rollback nginx-triage 2          (pin to a known-good earlier version)
  └─ same as :install nginx-triage@2
```

## 7. Error handling

| Failure                                              | Response / behaviour                                                            |
|------------------------------------------------------|----------------------------------------------------------------------------------|
| Auth missing / bad token                             | `401 { code: "unauth" }` — REPL prints "not registered with cc-control"          |
| Operator not admin, calls DELETE                     | `403 { code: "forbidden" }`                                                      |
| Schema violation on publish                          | `400 { code: "invalid_skill", field, reason }`                                   |
| Name not found on install                            | `404 { code: "not_found" }` — REPL prints "no such skill in registry"            |
| Version not found (`:install foo@99`)                | `404 { code: "version_not_found", available: [1,2,3] }`                          |
| cc-control unreachable                               | REPL: `cannot reach registry: <url>: <err>`. No partial state, nothing written.  |
| Atomic install: rename fails mid-write               | Temp file deleted; existing file (if any) untouched.                             |
| Tool whitelist references unknown tool               | `400 { field: "tools", reason: "unknown tool 'kubectl'" }` — publisher fixes.    |
| Concurrent publish race (two `:publish` same instant)| Both succeed; sqlite serialises via `BEGIN IMMEDIATE`. Author of vN = first commit. |
| Soft-deleted skill installed by name                 | Hidden from list/get; admin-only `?include_deleted=true` to view history.        |
| Already-installed skill on a deleted name            | Stays on the agent (deletion is not propagation; in-flight skills are preserved).|

The destructive-bash approval flow already establishes the
"surface error to model as text, let it retry" pattern; install errors
follow the same convention when triggered by an agent loop rather than
a human-typed REPL command.

## 8. Testing

### 8.1 `cc-control/internal/registry/`

- `store_test.go` — table-driven publish/list/get/history/rollback. Uses
  temp sqlite. One concurrency test: two goroutines hammer `Publish` for
  the same name; verify the `MAX(version)+1` race resolves with strictly
  monotonic versions and no INSERT failures.
- `http_test.go` — `httptest.NewServer` against the real handlers. Auth
  boundary: missing token → 401; agent token can publish + list + install
  but not delete; operator session can delete only with admin claim.
- `validate_test.go` — schema corner cases: long name, non-ASCII name,
  missing prompt, unknown tool, examples > 10, body > 64 KiB.

### 8.2 `cc-agent/internal/skills/`

- `client_test.go` — `httptest.NewServer` to fake cc-control. Tests publish
  round-trip; install writes to `team/`; atomic rename on partial failure
  (induce an `os.Rename` error); idempotent install (run twice, same result).
- Existing `reflect_test.go` stays. New cmd-level integration: `:reflect`
  produces a skill, then `:publish` ships it.

### 8.3 `cc-web/`

- One Playwright test: navigate `/skills`, search "nginx", click into list
  row, verify side panel renders, click "Install on host" with a stubbed
  cc-control. Verify the `install_request` reaches the stubbed agent.

### 8.4 Integration smoke (`tests/registry_e2e/`)

Spin up real cc-control + 2 cc-agent instances + a stub cc-web client.
Sequence:

1. agent-A `:reflect` → `:publish nginx-triage` → server returns v1.
2. agent-B `:registry` shows nginx-triage v1.
3. agent-B `:install nginx-triage` (auto-confirm in test) → file appears
   in agent-B's `skills_dir/team/`.
4. agent-A `:publish nginx-triage` again → v2.
5. agent-B `:install nginx-triage` → v2 replaces v1.
6. agent-B `:install nginx-triage@1` → rolls back to v1.
7. agent-A `:history nginx-triage` → shows both versions.

Roughly 80 lines of Go using the existing test harness.

## 9. Migration & rollout

- **Data migration:** none. `registry.db` is created on first start of
  cc-control if `registry_db_path` is set; absence = registry feature
  disabled.
- **Backward compatibility:** cc-agent without `control_http_url` set
  behaves exactly as today (no `:publish`/`:install`/etc.). Slash commands
  print "registry not configured" and exit early.
- **Rollout:** ship behind a single config knob. A team that doesn't want
  the registry simply leaves `registry_db_path` unset on cc-control. No
  schema changes to existing tables.
- **Removal path:** if v1 doesn't pan out, the registry package is
  self-contained — delete the directory, drop the table, remove the slash
  commands. No shared types leak into existing modules.

## 10. Open questions for v2

- Cryptographic signing: detached signatures verified at install time.
- Subscriptions / channels: agents `:subscribe` and receive push updates
  over the existing WS.
- Public registry: federation between orgs; anonymous browse; rate limits;
  moderation. This is where OCI-style content addressing (Approach C from
  brainstorming) becomes worth the ceremony.
- Per-skill model override (already on cc-agent roadmap): once a skill can
  pin a model, the registry should carry that pin.
- Skill telemetry: which skills are installed where, which fire most often.
  Useful but privacy-sensitive — opt-in only.

These are explicitly *not* on the v1 critical path.
