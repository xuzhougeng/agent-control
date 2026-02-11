#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT_PATH="$ROOT_DIR/scripts/cc-agent/test-e2e-approve-click-fix-case.sh"
PORT_ARG=""
SERVER_ID_ARG=""
UI_TOKEN_ARG=""
AGENT_TOKEN_ARG=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --port)
      PORT_ARG="${2:-}"
      shift 2
      ;;
    --server-id)
      SERVER_ID_ARG="${2:-}"
      shift 2
      ;;
    --ui-token)
      UI_TOKEN_ARG="${2:-}"
      shift 2
      ;;
    --agent-token)
      AGENT_TOKEN_ARG="${2:-}"
      shift 2
      ;;
    *)
      echo "unknown arg: $1" >&2
      echo "usage: $0 [--port N] [--server-id ID] [--ui-token TOKEN] [--agent-token TOKEN]" >&2
      exit 1
      ;;
  esac
done
PS_OUTPUT="$(ps -ax -o pid= -o command=)"

# Auto-discover cc-control runtime flags from process list:
#   -addr, -ui-token, -agent-token
# If --port is provided, prefer that exact port.
DISCOVERED_JSON="$(python3 - "$PS_OUTPUT" "$PORT_ARG" <<'PY'
import json
import re
import shlex
import sys

text = sys.argv[1]
preferred_port = (sys.argv[2] or "").strip()
best = None

for raw in text.splitlines():
    raw = raw.strip()
    if not raw:
        continue
    try:
        _pid, cmd = raw.split(" ", 1)
    except ValueError:
        continue
    if "cc-control" not in cmd:
        continue
    if " test-e2e-approve-existing-control.sh" in cmd:
        continue
    if "python" in cmd and "ps -ax" in cmd:
        continue
    try:
        parts = shlex.split(cmd)
    except ValueError:
        continue
    addr = ""
    ui_token = ""
    agent_token = ""
    for i, p in enumerate(parts):
        if p == "-addr" and i + 1 < len(parts):
            addr = parts[i + 1]
        elif p == "-ui-token" and i + 1 < len(parts):
            ui_token = parts[i + 1]
        elif p == "-agent-token" and i + 1 < len(parts):
            agent_token = parts[i + 1]
    m = re.search(r":(\d+)$", addr or "")
    port = m.group(1) if m else ""
    score = int(bool(addr)) + int(bool(ui_token)) + int(bool(agent_token))
    if preferred_port:
        if port == preferred_port:
            score += 100
        else:
            score -= 100
    if best is None or score > best["score"]:
        best = {
            "score": score,
            "addr": addr,
            "port": port,
            "ui_token": ui_token,
            "agent_token": agent_token,
        }

if best is None:
    print(json.dumps({"addr": "", "port": "", "ui_token": "", "agent_token": ""}))
else:
    print(json.dumps({
        "addr": best["addr"],
        "port": best["port"],
        "ui_token": best["ui_token"],
        "agent_token": best["agent_token"],
    }))
PY
)"

DISCOVERED_PORT="$(python3 - "$DISCOVERED_JSON" <<'PY'
import json
import sys

obj = json.loads(sys.argv[1])
print(obj.get("port") or "")
PY
)"

DISCOVERED_UI_TOKEN="$(python3 - "$DISCOVERED_JSON" <<'PY'
import json
import sys
obj = json.loads(sys.argv[1])
print(obj.get("ui_token") or "")
PY
)"

DISCOVERED_AGENT_TOKEN="$(python3 - "$DISCOVERED_JSON" <<'PY'
import json
import sys
obj = json.loads(sys.argv[1])
print(obj.get("agent_token") or "")
PY
)"

PORT_ENV_RAW="${PORT:-}"
UI_TOKEN_ENV_RAW="${UI_TOKEN:-}"
AGENT_TOKEN_ENV_RAW="${AGENT_TOKEN:-}"
ALLOW_ROOT_ENV_RAW="${ALLOW_ROOT:-}"

if [[ -n "$PORT_ARG" && -n "$DISCOVERED_PORT" && "$DISCOVERED_PORT" != "$PORT_ARG" ]]; then
  DISCOVERED_UI_TOKEN=""
  DISCOVERED_AGENT_TOKEN=""
fi

PORT="${PORT_ARG:-${PORT_ENV_RAW:-${DISCOVERED_PORT:-18080}}}"
UI_TOKEN="${UI_TOKEN_ARG:-${UI_TOKEN_ENV_RAW:-$DISCOVERED_UI_TOKEN}}"
AGENT_TOKEN="${AGENT_TOKEN_ARG:-${AGENT_TOKEN_ENV_RAW:-$DISCOVERED_AGENT_TOKEN}}"

if [[ -z "$PORT_ARG" && -z "$PORT_ENV_RAW" && -n "$DISCOVERED_PORT" ]]; then
  echo "[cc-agent][auto] PORT not explicitly provided; using discovered port: ${DISCOVERED_PORT}"
  echo "[cc-agent][auto] tip: pass --port 18080 to avoid targeting the wrong control"
fi

if [[ -z "$UI_TOKEN" ]]; then
  echo "cannot auto-discover UI_TOKEN for port ${PORT}; pass --ui-token <token>" >&2
  exit 1
fi

CONTROL_URL="http://127.0.0.1:${PORT}"
SERVER_ID_INPUT="${SERVER_ID_ARG:-${SERVER_ID:-}}"
SELECT_RESULT_JSON="$(python3 - "$CONTROL_URL" "$UI_TOKEN" "$SERVER_ID_INPUT" <<'PY'
import json
import sys
import urllib.error
import urllib.request

base, token, preferred = sys.argv[1:]
req = urllib.request.Request(base + "/api/servers", method="GET")
req.add_header("Authorization", f"Bearer {token}")
try:
    with urllib.request.urlopen(req, timeout=6) as resp:
        raw = resp.read()
        body = json.loads(raw) if raw else {}
except urllib.error.HTTPError as e:
    print(json.dumps({"error": f"HTTP_{e.code}", "server_id": "", "allow_root": ""}))
    raise SystemExit(0)
except Exception:
    print(json.dumps({"error": "HTTP_0", "server_id": "", "allow_root": ""}))
    raise SystemExit(0)

servers = body.get("servers", [])
online = [s for s in servers if s.get("status") == "online" and s.get("server_id")]
if preferred:
    for s in online:
        if s.get("server_id") == preferred:
            roots = s.get("allow_roots") or []
            allow_root = roots[0] if roots and roots[0] else ""
            print(json.dumps({"error": "", "server_id": preferred, "allow_root": allow_root}))
            raise SystemExit(0)
    print(json.dumps({"error": "MISSING_PREFERRED", "server_id": "", "allow_root": ""}))
    raise SystemExit(0)
if online:
    s = online[0]
    roots = s.get("allow_roots") or []
    allow_root = roots[0] if roots and roots[0] else ""
    print(json.dumps({"error": "", "server_id": s.get("server_id") or "", "allow_root": allow_root}))
else:
    print(json.dumps({"error": "", "server_id": "", "allow_root": ""}))
PY
)"

SELECTED_SERVER_ID="$(python3 - "$SELECT_RESULT_JSON" <<'PY'
import json
import sys
obj = json.loads(sys.argv[1])
print(obj.get("server_id") or "")
PY
)"

SELECTED_ALLOW_ROOT="$(python3 - "$SELECT_RESULT_JSON" <<'PY'
import json
import sys
obj = json.loads(sys.argv[1])
print(obj.get("allow_root") or "")
PY
)"

SELECT_ERROR="$(python3 - "$SELECT_RESULT_JSON" <<'PY'
import json
import sys
obj = json.loads(sys.argv[1])
print(obj.get("error") or "")
PY
)"

if [[ "$SELECT_ERROR" == HTTP_* ]]; then
  echo "cannot query ${CONTROL_URL}/api/servers (${SELECT_ERROR}); check --port/--ui-token" >&2
  exit 1
fi
if [[ "$SELECT_ERROR" == "MISSING_PREFERRED" ]]; then
  echo "preferred SERVER_ID=${SERVER_ID_INPUT} is not online" >&2
  exit 1
fi

if [[ -n "$SELECTED_SERVER_ID" ]]; then
  if [[ -z "$ALLOW_ROOT_ENV_RAW" && -n "$SELECTED_ALLOW_ROOT" ]]; then
    ALLOW_ROOT_ENV_RAW="$SELECTED_ALLOW_ROOT"
    echo "[cc-agent][auto] using allow_root as cwd: ${ALLOW_ROOT_ENV_RAW}"
  fi
  echo "[cc-agent][auto] using existing online server: ${SELECTED_SERVER_ID}"
  PORT="$PORT" UI_TOKEN="$UI_TOKEN" SERVER_ID="$SELECTED_SERVER_ID" ALLOW_ROOT="$ALLOW_ROOT_ENV_RAW" \
  USE_EXISTING_CONTROL=1 USE_EXISTING_AGENT=1 \
  bash "$SCRIPT_PATH"
  exit $?
fi

if [[ -z "$AGENT_TOKEN" ]]; then
  echo "no online server and AGENT_TOKEN not discovered; set AGENT_TOKEN or bring an agent online" >&2
  exit 1
fi

SERVER_ID_AUTO="${SERVER_ID_INPUT:-srv-e2e-auto-$(date +%s)}"
echo "[cc-agent][auto] no online server, starting temporary agent: ${SERVER_ID_AUTO}"

PORT="$PORT" UI_TOKEN="$UI_TOKEN" AGENT_TOKEN="$AGENT_TOKEN" SERVER_ID="$SERVER_ID_AUTO" ALLOW_ROOT="$ALLOW_ROOT_ENV_RAW" \
USE_EXISTING_CONTROL=1 USE_EXISTING_AGENT=0 \
bash "$SCRIPT_PATH"
