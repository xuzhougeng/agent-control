# cc-agent test scripts

## Quick run

```bash
bash scripts/cc-agent/test-all.sh
```

## Scripts

- `test-unit.sh`: runs `go test` in `cc-agent`.
- `test-integration-local.sh`: starts local `cc-control` + `cc-agent`, then validates:
  - agent registers and server becomes online
  - session can start/stop with allowed cwd
  - disallowed cwd is rejected by agent policy (`reject_cwd`)
- `test-e2e-approve-click-fix-case.sh`: full e2e check for approval flow with command
  `create file approve_click_fix_case\r`:
  - waits for `approval_needed`
  - sends `action: approve` (same as UI click path)
  - asserts `awaiting_approval=false` and event resolved
- `test-all.sh`: runs unit + integration in sequence.

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
