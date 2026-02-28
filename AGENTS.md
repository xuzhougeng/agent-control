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

## Web UI Expectations
- Desktop-first: prioritize the desktop web workspace before mobile polish. Mobile should remain functional, but primary design decisions should optimize desktop information hierarchy and task flow.
- Treat `/` as the single real workspace entrypoint. `/chat` is compatibility-only and should not regain separate product logic.
- Prefer a clear workspace model: left rail for navigation/creation, center for the active task, right rail for secondary context such as approvals and runtime history.
- Avoid generic admin-tool aesthetics. UI changes should establish a coherent visual language with deliberate typography, spacing, color hierarchy, and panel structure.
- Do not add UI complexity unless it improves the main task flow: select session, inspect status, switch mode, continue work.

## Testing Guidelines
- Framework: Go standard `testing` package.
- Location: tests live alongside code (for example `cc-control/internal/.../*_test.go`).
- Naming: `TestXxx` functions in `*_test.go` files.
- Targeted runs: `go test ./cc-agent/...` or `go test ./cc-control/...`.
- For web changes, run the Playwright E2E suite when practical: `npm run test:web:e2e`.
- Keep Playwright screenshots enabled so reviewers can inspect the resulting UI state. The default artifact location is `test-results/`.

## Commit & Pull Request Guidelines
- Commit messages follow Conventional Commits seen in history, for example `feat: ...`, `docs: ...`, `fix: ...`.
- PRs should include a clear summary, tests run (or note if not run), and screenshots for UI changes.
- Link relevant issues or deployment notes when behavior or APIs change.

## Security & Configuration Tips
- Do not commit tokens or audit logs; prefer env vars or local flags.
- For local runs, set `-admin-token`, `-ui-dir`, and `-audit-path` explicitly; see `README.md` examples.
