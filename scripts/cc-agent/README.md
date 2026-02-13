# cc-agent test scripts

## Quick run

```bash
bash scripts/cc-agent/test-all.sh
```

Run with e2e approve check:

```bash
RUN_E2E_APPROVE=1 bash scripts/cc-agent/test-all.sh
```

## Scripts

- `test-unit.sh`: runs `go test` in `cc-agent`.
- `test-integration-local.sh`: starts local `cc-control` + `cc-agent`, then validates:
  - agent registers and server becomes online
  - session can start/stop with allowed cwd
  - disallowed cwd is rejected by agent policy (`reject_cwd`)
- `test-e2e-approve-click-fix-case.sh`: full e2e check for approval flow with command
  `create file approve_click_fix_case\r`:
  - starts `cc-control` with `-enable-prompt-detection` (required for `approval_needed`)
  - waits for `approval_needed`
  - sends `action: approve` (same as UI click path)
  - asserts `awaiting_approval=false` and event resolved
- `test-e2e-approve-existing-control.sh`: same e2e check, but connects to an already
  running `cc-control` and reuses an existing online agent
  (`USE_EXISTING_CONTROL=1`, `USE_EXISTING_AGENT=1`).
  - make sure the existing `cc-control` was started with `-enable-prompt-detection`
  - auto-discovers `PORT` / `UI_TOKEN` / `AGENT_TOKEN` from running `cc-control`
    process args when possible
  - auto-selects an online `SERVER_ID` if not provided
  - if no online server is available, it auto-starts a temporary agent (when
    `AGENT_TOKEN` is available)
  - if `ALLOW_ROOT` is unset in that branch, defaults to repo root
- `test-all.sh`: runs unit + integration in sequence.
  - set `RUN_E2E_APPROVE=1` to include approve e2e.

## Useful env overrides

```bash
PORT=18091 \
AGENT_TOKEN=agent-test-token \
UI_TOKEN=admin-test-token \
SERVER_ID=srv-agent-it \
ALLOW_ROOT=/absolute/allowed/root \
CLAUDE_PATH=/bin/sh \
bash scripts/cc-agent/test-integration-local.sh
```

For the approve-click e2e script (real Claude command), you usually want real Claude path:

```bash
CLAUDE_PATH=/opt/homebrew/bin/claude \
bash scripts/cc-agent/test-e2e-approve-click-fix-case.sh
```

If `cc-control` is already running (for example on port `18080`), run:

```bash
PORT=18080 \
CLAUDE_PATH=/opt/homebrew/bin/claude \
bash scripts/cc-agent/test-e2e-approve-existing-control.sh
```

Fully automatic (best effort auto-discovery from running `cc-control`):

```bash
bash scripts/cc-agent/test-e2e-approve-existing-control.sh
```

Force exact target control port (recommended):

```bash
bash scripts/cc-agent/test-e2e-approve-existing-control.sh --port 18080
```

If there are multiple online servers, pin one:

```bash
PORT=18080 \
SERVER_ID=srv-local \
CLAUDE_PATH=/opt/homebrew/bin/claude \
bash scripts/cc-agent/test-e2e-approve-existing-control.sh
```
