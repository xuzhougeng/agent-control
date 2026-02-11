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

PORT="${PORT:-18082}"
CONTROL_URL="http://127.0.0.1:${PORT}"
CONTROL_WS_AGENT_URL="ws://127.0.0.1:${PORT}/ws/agent"
CONTROL_WS_CLIENT_URL="ws://127.0.0.1:${PORT}/ws/client"

AGENT_TOKEN="${AGENT_TOKEN:-agent-test-token}"
UI_TOKEN="${UI_TOKEN:-admin-test-token}"
SERVER_ID="${SERVER_ID:-srv-e2e-approve-fix}"
ALLOW_ROOT="${ALLOW_ROOT:-$ROOT_DIR}"
CLAUDE_PATH="${CLAUDE_PATH:-/opt/homebrew/bin/claude}"

COMMAND_TEXT="${COMMAND_TEXT:-create file approve_click_fix_case\\r}"
APPROVAL_TIMEOUT_SEC="${APPROVAL_TIMEOUT_SEC:-240}"
ACTION_TIMEOUT_SEC="${ACTION_TIMEOUT_SEC:-45}"
STARTUP_TIMEOUT_SEC="${STARTUP_TIMEOUT_SEC:-60}"

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/cc-agent-e2e-approve.XXXXXX")"
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

if lsof -nP -iTCP:"${PORT}" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "port ${PORT} already in use; set PORT to another value" >&2
  exit 1
fi

echo "[cc-agent][e2e] logs: ${TMP_DIR}"
echo "[cc-agent][e2e] starting cc-control on :${PORT}"
go -C "$ROOT_DIR/cc-control" run ./cmd/cc-control \
  -addr ":${PORT}" \
  -ui-dir ../ui \
  -agent-token "$AGENT_TOKEN" \
  -ui-token "$UI_TOKEN" \
  -audit-path "$AUDIT_PATH" \
  >"$CONTROL_LOG" 2>&1 &
CONTROL_PID="$!"

echo "[cc-agent][e2e] starting cc-agent (server_id=${SERVER_ID})"
go -C "$ROOT_DIR/cc-agent" run ./cmd/cc-agent \
  -control-url "$CONTROL_WS_AGENT_URL" \
  -agent-token "$AGENT_TOKEN" \
  -server-id "$SERVER_ID" \
  -allow-root "$ALLOW_ROOT" \
  -claude-path "$CLAUDE_PATH" \
  >"$AGENT_LOG" 2>&1 &
AGENT_PID="$!"

set +e
python3 - \
  "$CONTROL_URL" \
  "$CONTROL_WS_CLIENT_URL" \
  "$UI_TOKEN" \
  "$SERVER_ID" \
  "$ALLOW_ROOT" \
  "$COMMAND_TEXT" \
  "$APPROVAL_TIMEOUT_SEC" \
  "$ACTION_TIMEOUT_SEC" \
  "$STARTUP_TIMEOUT_SEC" <<'PY'
import base64
import json
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

try:
    import websocket  # websocket-client
except Exception as e:  # pragma: no cover
    raise SystemExit(
        "python websocket-client is required. install with: "
        "python3 -m pip install websocket-client\n"
        f"import error: {e}"
    )

(
    control_url,
    ws_client_base,
    ui_token,
    server_id,
    allow_root,
    command_text_raw,
    approval_timeout_sec,
    action_timeout_sec,
    startup_timeout_sec,
) = sys.argv[1:]
approval_timeout_sec = int(approval_timeout_sec)
action_timeout_sec = int(action_timeout_sec)
startup_timeout_sec = int(startup_timeout_sec)

def request_json(method, path, body=None, timeout=8):
    req = urllib.request.Request(control_url + path, method=method)
    req.add_header("Authorization", f"Bearer {ui_token}")
    if body is not None:
        req.add_header("Content-Type", "application/json")
        data = json.dumps(body).encode()
    else:
        data = None
    try:
        with urllib.request.urlopen(req, data=data, timeout=timeout) as resp:
            raw = resp.read()
            return resp.status, (json.loads(raw) if raw else {})
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

def create_session():
    status, body = request_json("POST", "/api/sessions", {
        "server_id": server_id,
        "cwd": allow_root,
        "env": {"CC_PROFILE": "e2e"},
        "cols": 120,
        "rows": 30,
    })
    if status != 201:
        raise RuntimeError(f"create session failed status={status} body={body}")
    sid = body.get("session_id")
    if not sid:
        raise RuntimeError(f"missing session_id in response: {body}")
    return sid

def get_session(session_id):
    q = urllib.parse.quote(server_id, safe="")
    status, body = request_json("GET", f"/api/sessions?server_id={q}")
    if status != 200:
        raise RuntimeError(f"list sessions failed status={status} body={body}")
    for s in body.get("sessions", []):
        if s.get("session_id") == session_id:
            return s
    return None

def decode_command(raw):
    cmd = raw
    if "\\r" in cmd or "\\n" in cmd or "\\t" in cmd:
        cmd = cmd.encode("utf-8").decode("unicode_escape")
    if not cmd.endswith("\r"):
        cmd += "\r"
    return cmd

if not wait_server_online(startup_timeout_sec):
    raise RuntimeError(f"server {server_id} did not become online in {startup_timeout_sec}s")
print(f"[python] server online: {server_id}")

session_id = create_session()
print(f"[python] created session: {session_id}")

ws_url = f"{ws_client_base}?token={urllib.parse.quote(ui_token, safe='')}"
ws = websocket.create_connection(ws_url, timeout=12)
ws.settimeout(2)

def recv_json(timeout=2):
    ws.settimeout(timeout)
    try:
        return json.loads(ws.recv())
    except websocket.WebSocketTimeoutException:
        return None

# drain initial server messages (debug_probe, global replays, etc.)
for _ in range(5):
    _ = recv_json(timeout=0.2)

ws.send(json.dumps({
    "type": "attach",
    "data": {"session_id": session_id, "since_seq": 0},
}))

# Let Claude session start rendering.
start = time.time()
while time.time() - start < 8:
    _ = recv_json(timeout=0.5)

command = decode_command(command_text_raw)
ws.send(json.dumps({
    "type": "term_in",
    "session_id": session_id,
    "data_b64": base64.b64encode(command.encode()).decode(),
}))
print(f"[python] sent command: {command_text_raw!r}")

approval_event = None
deadline = time.time() + approval_timeout_sec
last_error = None
while time.time() < deadline:
    msg = recv_json(timeout=2)
    if msg is None:
        continue
    if msg.get("type") == "error":
        last_error = msg
        continue
    if msg.get("type") == "event":
        ev = msg.get("data", {})
        if ev.get("kind") == "approval_needed" and ev.get("session_id") == session_id:
            approval_event = ev
            break

if approval_event is None:
    raise RuntimeError(
        f"approval event not observed in {approval_timeout_sec}s; "
        f"last_error={last_error}"
    )
print(f"[python] approval event received: {approval_event.get('event_id')}")

# Mimic UI click behavior: action approve without event_id.
ws.send(json.dumps({
    "type": "action",
    "session_id": session_id,
    "data": {"kind": "approve"},
}))
print("[python] sent action: approve")

approved = False
deadline = time.time() + action_timeout_sec
while time.time() < deadline:
    msg = recv_json(timeout=2)
    if msg is None:
        continue
    if msg.get("type") == "error" and msg.get("session_id") in ("", session_id):
        raise RuntimeError(f"server returned error after approve: {msg}")
    if msg.get("type") == "session_update":
        data = msg.get("data", {})
        if data.get("session_id") == session_id and data.get("awaiting_approval") is False:
            approved = True
            break

if not approved:
    raise RuntimeError(f"awaiting_approval did not clear in {action_timeout_sec}s")

session = get_session(session_id)
if session is None:
    raise RuntimeError("session disappeared from list")
if session.get("awaiting_approval") is not False:
    raise RuntimeError(f"rest check failed: awaiting_approval={session.get('awaiting_approval')}")

status, events_body = request_json("GET", f"/api/sessions/{session_id}/events")
if status == 200:
    target = None
    for ev in events_body.get("events", []):
        if ev.get("event_id") == approval_event.get("event_id"):
            target = ev
            break
    if target is None:
        raise RuntimeError("approval event missing from session events")
    if not target.get("resolved", False):
        raise RuntimeError(f"approval event should be resolved, got: {target}")

ws.close()
print("[python] e2e approve flow passed")
PY
status=$?
set -e

if [[ $status -ne 0 ]]; then
  print_logs_and_exit "$status"
fi

echo "[cc-agent][e2e] PASS"
echo "[cc-agent][e2e] logs: ${TMP_DIR}"
