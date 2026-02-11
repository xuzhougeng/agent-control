#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

if ! command -v go >/dev/null 2>&1; then
  echo "go not found in PATH" >&2
  exit 1
fi
if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 not found in PATH" >&2
  exit 1
fi

PORT="${PORT:-18081}"
CONTROL_URL="http://127.0.0.1:${PORT}"
CONTROL_WS_URL="ws://127.0.0.1:${PORT}/ws/agent"
AGENT_TOKEN="${AGENT_TOKEN:-agent-test-token}"
UI_TOKEN="${UI_TOKEN:-admin-test-token}"
SERVER_ID="${SERVER_ID:-srv-agent-it-local}"
ALLOW_ROOT="${ALLOW_ROOT:-$ROOT_DIR}"
CLAUDE_PATH="${CLAUDE_PATH:-/bin/sh}"
HEARTBEAT_WAIT_SEC="${HEARTBEAT_WAIT_SEC:-45}"

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/cc-agent-it.XXXXXX")"
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

echo "[cc-agent][integration] logs: ${TMP_DIR}"
echo "[cc-agent][integration] starting cc-control on :${PORT}"
go -C "$ROOT_DIR/cc-control" run ./cmd/cc-control \
  -addr ":${PORT}" \
  -ui-dir ../ui \
  -agent-token "$AGENT_TOKEN" \
  -ui-token "$UI_TOKEN" \
  -audit-path "$AUDIT_PATH" \
  >"$CONTROL_LOG" 2>&1 &
CONTROL_PID="$!"

echo "[cc-agent][integration] starting cc-agent (server_id=${SERVER_ID})"
go -C "$ROOT_DIR/cc-agent" run ./cmd/cc-agent \
  -control-url "$CONTROL_WS_URL" \
  -agent-token "$AGENT_TOKEN" \
  -server-id "$SERVER_ID" \
  -allow-root "$ALLOW_ROOT" \
  -claude-path "$CLAUDE_PATH" \
  >"$AGENT_LOG" 2>&1 &
AGENT_PID="$!"

set +e
python3 - "$CONTROL_URL" "$UI_TOKEN" "$SERVER_ID" "$ALLOW_ROOT" "$HEARTBEAT_WAIT_SEC" <<'PY'
import json
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request

base, token, server_id, allow_root, wait_sec = sys.argv[1:]
wait_sec = int(wait_sec)

def request_json(method, path, body=None, timeout=5):
    req = urllib.request.Request(base + path, method=method)
    req.add_header("Authorization", f"Bearer {token}")
    if body is not None:
        req.add_header("Content-Type", "application/json")
        data = json.dumps(body).encode()
    else:
        data = None
    try:
        with urllib.request.urlopen(req, data=data, timeout=timeout) as resp:
            raw = resp.read()
            if not raw:
                return resp.status, {}
            return resp.status, json.loads(raw)
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
            for s in body.get("servers", []):
                if s.get("server_id") == server_id and s.get("status") == "online":
                    return True
        time.sleep(1)
    return False

def create_session(cwd):
    status, body = request_json("POST", "/api/sessions", {
        "server_id": server_id,
        "cwd": cwd,
        "env": {"CC_PROFILE": "test"},
        "cols": 100,
        "rows": 30,
    })
    if status != 201:
        raise RuntimeError(f"create session failed status={status} body={body}")
    sid = body.get("session_id")
    if not sid:
        raise RuntimeError(f"create session missing session_id body={body}")
    return sid

def get_session(sid):
    q = urllib.parse.quote(server_id, safe="")
    status, body = request_json("GET", f"/api/sessions?server_id={q}")
    if status != 200:
        raise RuntimeError(f"list sessions failed status={status} body={body}")
    for s in body.get("sessions", []):
        if s.get("session_id") == sid:
            return s
    return None

def wait_session_status(sid, allowed, timeout_sec):
    deadline = time.time() + timeout_sec
    last = None
    while time.time() < deadline:
        s = get_session(sid)
        if s is not None:
            last = s
            if s.get("status") in allowed:
                return s
        time.sleep(1)
    raise RuntimeError(f"session {sid} did not reach {allowed}, last={last}")

def stop_session(sid):
    status, body = request_json("POST", f"/api/sessions/{sid}/stop", {})
    if status != 200:
        raise RuntimeError(f"stop session failed status={status} body={body}")

if not wait_server_online(wait_sec):
    raise RuntimeError(f"server {server_id} did not become online in {wait_sec}s")
print(f"[python] server online: {server_id}")

# Case 1: allowed cwd should start and stop cleanly.
sid_ok = create_session(allow_root)
print(f"[python] created allowed session: {sid_ok}")
s_ok = wait_session_status(sid_ok, {"running", "error"}, 30)
if s_ok.get("status") != "running":
    raise RuntimeError(f"allowed session should be running, got {s_ok}")
stop_session(sid_ok)
s_stopped = wait_session_status(sid_ok, {"exited", "error"}, 30)
if s_stopped.get("status") != "exited":
    raise RuntimeError(f"stopped session expected exited, got {s_stopped}")
print(f"[python] stop flow ok: {sid_ok}")

# Case 2: disallowed cwd should be rejected by cc-agent path policy.
bad_cwd = tempfile.mkdtemp(prefix="cc-agent-badcwd-")
sid_bad = create_session(bad_cwd)
print(f"[python] created disallowed session: {sid_bad} cwd={bad_cwd}")
s_bad = wait_session_status(sid_bad, {"error", "running"}, 30)
if s_bad.get("status") != "error":
    raise RuntimeError(f"disallowed cwd should fail, got {s_bad}")
reason = s_bad.get("exit_reason", "")
if "reject_cwd" not in reason:
    raise RuntimeError(f"expected reject_cwd in exit_reason, got: {reason!r}")
print(f"[python] disallowed cwd rejected: {sid_bad} reason={reason}")

print("[python] integration checks passed")
PY
status=$?
set -e

if [[ $status -ne 0 ]]; then
  print_logs_and_exit "$status"
fi

echo "[cc-agent][integration] PASS"
echo "[cc-agent][integration] logs: ${TMP_DIR}"
