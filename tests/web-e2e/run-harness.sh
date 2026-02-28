#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PORT="${CC_WEB_E2E_PORT:-18110}"
CONTROL_WS_URL="ws://127.0.0.1:${PORT}/ws/agent"
UI_TOKEN="${CC_WEB_E2E_UI_TOKEN:-ui-e2e-token}"
AGENT_TOKEN="${CC_WEB_E2E_AGENT_TOKEN:-agent-e2e-token}"
SERVER_ID="${CC_WEB_E2E_SERVER_ID:-srv-e2e}"
ALLOW_ROOT="${CC_WEB_E2E_ALLOW_ROOT:-$ROOT_DIR}"
PRESEEDED_SESSION_ID="${CC_WEB_E2E_PRESEEDED_SESSION_ID:-11111111-1111-4111-8111-111111111111}"

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/cc-web-e2e.XXXXXX")"
BIN_DIR="${TMP_DIR}/bin"
HOME_DIR="${TMP_DIR}/home"
CONTROL_LOG="${TMP_DIR}/cc-control.log"
AGENT_LOG="${TMP_DIR}/cc-agent.log"
AUDIT_PATH="${TMP_DIR}/audit.jsonl"
FAKE_CLAUDE="${ROOT_DIR}/tests/web-e2e/fixtures/fake-claude.py"

mkdir -p "$BIN_DIR" "$HOME_DIR/.claude/session-env"
printf 'fake-session\n' > "$HOME_DIR/.claude/session-env/$PRESEEDED_SESSION_ID"

cleanup() {
  if [[ -n "${AGENT_PID:-}" ]]; then
    kill "${AGENT_PID}" 2>/dev/null || true
  fi
  if [[ -n "${CONTROL_PID:-}" ]]; then
    kill "${CONTROL_PID}" 2>/dev/null || true
  fi
}
trap cleanup EXIT

go -C "${ROOT_DIR}/cc-control" build -o "${BIN_DIR}/cc-control" ./cmd/cc-control
go -C "${ROOT_DIR}/cc-agent" build -o "${BIN_DIR}/cc-agent" ./cmd/cc-agent
go -C "${ROOT_DIR}/cc-agent" build -o "${BIN_DIR}/cc-chat-claude" ./cmd/cc-chat-claude

HOME="${HOME_DIR}" "${BIN_DIR}/cc-control" \
  -addr ":${PORT}" \
  -ui-dir "${ROOT_DIR}/cc-web" \
  -ui-token "${UI_TOKEN}" \
  -agent-token "${AGENT_TOKEN}" \
  -audit-path "${AUDIT_PATH}" \
  >"${CONTROL_LOG}" 2>&1 &
CONTROL_PID="$!"

HOME="${HOME_DIR}" \
CC_CLAUDE_CMD="${FAKE_CLAUDE}" \
"${BIN_DIR}/cc-agent" \
  -control-url "${CONTROL_WS_URL}" \
  -agent-token "${AGENT_TOKEN}" \
  -server-id "${SERVER_ID}" \
  -allow-root "${ALLOW_ROOT}" \
  -claude-path "${FAKE_CLAUDE}" \
  -chat-worker "${BIN_DIR}/cc-chat-claude" \
  >"${AGENT_LOG}" 2>&1 &
AGENT_PID="$!"

python3 - "${PORT}" "${SERVER_ID}" "${UI_TOKEN}" <<'PY'
import json
import sys
import time
import urllib.error
import urllib.request

port, server_id, token = sys.argv[1:]
base = f"http://127.0.0.1:{port}"
deadline = time.time() + 45

def request_json(path):
    req = urllib.request.Request(base + path)
    req.add_header("Authorization", f"Bearer {token}")
    with urllib.request.urlopen(req, timeout=2) as resp:
        raw = resp.read()
        return json.loads(raw) if raw else {}

while time.time() < deadline:
    try:
        body = request_json("/api/servers")
    except Exception:
        time.sleep(0.5)
        continue
    for server in body.get("servers", []):
        if server.get("server_id") == server_id and server.get("status") == "online":
            sys.exit(0)
    time.sleep(0.5)

raise SystemExit("server did not become online")
PY

echo "[web-e2e] harness ready on :${PORT}"

while kill -0 "${CONTROL_PID}" 2>/dev/null && kill -0 "${AGENT_PID}" 2>/dev/null; do
  sleep 1
done

echo "[web-e2e] harness stopped unexpectedly" >&2
exit 1
