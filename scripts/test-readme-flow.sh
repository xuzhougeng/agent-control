#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if ! command -v go >/dev/null 2>&1; then
  echo "go not found in PATH" >&2
  exit 1
fi
if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 not found in PATH" >&2
  exit 1
fi

PORT="${PORT:-18080}"
CONTROL_HTTP="http://127.0.0.1:${PORT}"
CONTROL_WS="ws://127.0.0.1:${PORT}/ws/agent"

ADMIN_TOKEN="${ADMIN_TOKEN:-admin-dev-token}"
UI_TOKEN="${UI_TOKEN:-ui-dev-token}"
AGENT_TOKEN="${AGENT_TOKEN:-agent-dev-token}"
SERVER_ID="${SERVER_ID:-srv-local}"
ALLOW_ROOT="${ALLOW_ROOT:-$ROOT_DIR}"
CLAUDE_PATH="${CLAUDE_PATH:-/bin/sh}"
WAIT_SEC="${WAIT_SEC:-45}"

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/cc-readme-flow.XXXXXX")"
CONTROL_LOG="${TMP_DIR}/cc-control.log"
AGENT_LOG="${TMP_DIR}/cc-agent.log"
AUDIT_PATH="${TMP_DIR}/audit.jsonl"

CONTROL_PID=""
AGENT_PID=""

print_logs_and_exit() {
  local code="${1:-1}"
  echo ""
  echo "========== cc-control log =========="
  if [[ -f "$CONTROL_LOG" ]]; then
    tail -n 200 "$CONTROL_LOG" || true
  fi
  echo "========== cc-agent log =========="
  if [[ -f "$AGENT_LOG" ]]; then
    tail -n 200 "$AGENT_LOG" || true
  fi
  echo "logs dir: ${TMP_DIR}"
  exit "$code"
}

cleanup() {
  if [[ -n "$AGENT_PID" ]]; then
    kill "$AGENT_PID" 2>/dev/null || true
  fi
  if [[ -n "$CONTROL_PID" ]]; then
    kill "$CONTROL_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

echo "[readme-flow] logs: ${TMP_DIR}"
echo "[readme-flow] starting cc-control on :${PORT}"
go -C "$ROOT_DIR/cc-control" run ./cmd/cc-control \
  -addr ":${PORT}" \
  -ui-dir ../ui \
  -admin-token "$ADMIN_TOKEN" \
  -agent-token "$AGENT_TOKEN" \
  -ui-token "$UI_TOKEN" \
  -audit-path "$AUDIT_PATH" \
  >"$CONTROL_LOG" 2>&1 &
CONTROL_PID="$!"

echo "[readme-flow] starting cc-agent (server_id=${SERVER_ID})"
go -C "$ROOT_DIR/cc-agent" run ./cmd/cc-agent \
  -control-url "$CONTROL_WS" \
  -agent-token "$AGENT_TOKEN" \
  -server-id "$SERVER_ID" \
  -allow-root "$ALLOW_ROOT" \
  -claude-path "$CLAUDE_PATH" \
  >"$AGENT_LOG" 2>&1 &
AGENT_PID="$!"

set +e
python3 - "$CONTROL_HTTP" "$UI_TOKEN" "$SERVER_ID" "$ALLOW_ROOT" "$WAIT_SEC" <<'PY'
import json
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

base, token, server_id, allow_root, wait_sec = sys.argv[1:]
wait_sec = int(wait_sec)

def request_json(method, path, body=None, timeout=5):
    req = urllib.request.Request(base + path, method=method)
    req.add_header("Authorization", f"Bearer {token}")
    data = None
    if body is not None:
        req.add_header("Content-Type", "application/json")
        data = json.dumps(body).encode()
    try:
        with urllib.request.urlopen(req, data=data, timeout=timeout) as resp:
            raw = resp.read()
            return resp.status, json.loads(raw) if raw else {}
    except urllib.error.HTTPError as e:
        raw = e.read()
        txt = raw.decode(errors="replace") if raw else ""
        return e.code, {"_error": txt}
    except urllib.error.URLError as e:
        return 0, {"_error": str(e)}

def wait_server_online(timeout_sec):
    deadline = time.time() + timeout_sec
    while time.time() < deadline:
        status, body = request_json("GET", "/api/servers")
        if status == 200:
            for server in body.get("servers", []):
                if server.get("server_id") == server_id and server.get("status") == "online":
                    return True
        time.sleep(1)
    return False

def find_session(session_id):
    status, body = request_json("GET", f"/api/sessions?server_id={urllib.parse.quote(server_id, safe='')}")
    if status != 200:
        raise RuntimeError(f"list sessions failed status={status} body={body}")
    for session in body.get("sessions", []):
        if session.get("session_id") == session_id:
            return session
    return None

def wait_session_status(session_id, allowed, timeout_sec):
    deadline = time.time() + timeout_sec
    last = None
    while time.time() < deadline:
        current = find_session(session_id)
        if current is not None:
            last = current
            if current.get("status") in allowed:
                return current
        time.sleep(1)
    raise RuntimeError(f"session {session_id} did not reach {allowed}, last={last}")

if not wait_server_online(wait_sec):
    raise RuntimeError(f"server {server_id} not online in {wait_sec}s")
print(f"[python] server online: {server_id}")

status, created = request_json("POST", "/api/sessions", {
    "server_id": server_id,
    "cwd": allow_root,
    "env": {},
    "cols": 120,
    "rows": 30,
})
if status != 201:
    raise RuntimeError(f"create session failed status={status} body={created}")
session_id = created.get("session_id")
if not session_id:
    raise RuntimeError(f"missing session_id in create response: {created}")
print(f"[python] session created: {session_id}")

running = wait_session_status(session_id, {"running", "error"}, 30)
if running.get("status") != "running":
    raise RuntimeError(f"session should be running, got {running}")
print(f"[python] session running: {session_id}")

status, body = request_json("POST", f"/api/sessions/{session_id}/stop", {})
if status != 200:
    raise RuntimeError(f"stop session failed status={status} body={body}")
print(f"[python] session stop requested: {session_id}")

stopped = wait_session_status(session_id, {"exited", "error"}, 30)
if stopped.get("status") != "exited":
    raise RuntimeError(f"session should be exited after stop, got {stopped}")
print(f"[python] session exited: {session_id}")

status, body = request_json("DELETE", f"/api/sessions/{session_id}", {})
if status != 200:
    raise RuntimeError(f"delete session failed status={status} body={body}")
print(f"[python] session deleted: {session_id}")

print("[python] readme flow PASS")
PY
status=$?
set -e

if [[ $status -ne 0 ]]; then
  print_logs_and_exit "$status"
fi

echo "[readme-flow] PASS"
echo "[readme-flow] logs: ${TMP_DIR}"
