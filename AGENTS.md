# Repository Guidelines

## Project Structure & Module Organization
- `cc-control/`: Go control plane service (REST/WS, token management, audit logging).
- `cc-agent/`: Go agent that connects to control plane and spawns PTYs.
- `cc-web/`: static browser UI (`index.html`, `admin.html`, `tenant.html`, `app.js`).
- `app/AgentControlMac/`: native macOS/iOS client (Xcode project).
- `docs/`: architecture, API, and deployment guides.
- `skills`: useful skill for agent.
- `scripts/`: test and integration scripts.
- `go.work`: workspace wiring for `cc-control` and `cc-agent`.

## Build, Test, and Development Commands
- `go test ./...` from repo root: run all Go tests across the workspace.
- `go run ./cmd/cc-control ...` from `cc-control/`: start the control plane (see `README.md` for flags).
- `go run ./cmd/cc-agent ...` from `cc-agent/`: start the agent and connect to control plane.
- `bash scripts/test-readme-flow.sh`: smoke test of the README flow (starts control + agent).
- `bash scripts/cc-agent/test-all.sh`: cc-agent unit + integration; add `RUN_E2E_APPROVE=1` for approval e2e.

## Coding Style & Naming Conventions
- Go: format with `gofmt`; package names are short/lowercase; exported identifiers use `CamelCase` and unexported use `camelCase`.
- Files: Go source follows existing `snake_case.go` patterns; tests use `*_test.go`.
- UI: keep changes scoped to `cc-web/` and follow the existing vanilla JS/DOM style (no framework).

## Testing Guidelines
- Framework: Go standard `testing` package.
- Location: tests live alongside code (for example `cc-control/internal/.../*_test.go`).
- Naming: `TestXxx` functions in `*_test.go` files.
- Targeted runs: `go test ./cc-agent/...` or `go test ./cc-control/...`.

## Commit & Pull Request Guidelines
- Commit messages follow Conventional Commits seen in history, for example `feat: ...`, `docs: ...`, `fix: ...`.
- PRs should include a clear summary, tests run (or note if not run), and screenshots for UI changes.
- Link relevant issues or deployment notes when behavior or APIs change.

## Security & Configuration Tips
- Do not commit tokens or audit logs; prefer env vars or local flags.
- For local runs, set `-admin-token`, `-ui-dir`, and `-audit-path` explicitly; see `README.md` examples.

## Cursor Cloud specific instructions

- **Go 1.25** is installed at `/usr/local/go/bin/go`. The update script ensures Go modules are downloaded on each startup.
- **Test command caveat**: `go test ./...` from repo root fails because the `go.work` workspace does not match `./...`. Use `go test ./cc-control/... ./cc-agent/...` instead.
- **Lint**: run `gofmt -l ./cc-control/ ./cc-agent/` to check formatting. Note: some existing files have format diffs — do not reformat unless explicitly asked.
- **No external services needed**: no databases, Docker, or Node.js required. Everything runs self-contained.
- **Starting services** (see `README.md` Quick Start for full flags):
  - cc-control: `cd cc-control && go run ./cmd/cc-control -addr :18080 -ui-dir ../cc-web -admin-token admin-dev-token -ui-token "" -agent-token "" -audit-path /tmp/audit.jsonl`
  - cc-agent: `cd cc-agent && go run ./cmd/cc-agent -control-url ws://127.0.0.1:18080/ws/agent -agent-token <TOKEN> -server-id srv-local -allow-root /workspace -claude-path /bin/sh`
  - Use `/bin/sh` as `-claude-path` for local testing when no AI CLI is available.
- **Token flow**: create tenant token via `POST /admin/tokens`, then issue UI+agent tokens via `POST /tenant/tokens`. See `README.md` for curl examples.
