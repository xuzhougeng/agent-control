#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -d "$SCRIPT_DIR/bin" && -d "$SCRIPT_DIR/cc-web" ]]; then
  BUNDLE_ROOT="$SCRIPT_DIR"
else
  BUNDLE_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

CONTROL_PORT="${CONTROL_PORT:-18080}"
ADMIN_TOKEN="${ADMIN_TOKEN:-admin-dev-token}"
SERVER_ID_PREFIX="${SERVER_ID_PREFIX:-${SERVER_ID:-srv-linux}}"
PTY_SERVER_ID="${PTY_SERVER_ID:-${SERVER_ID_PREFIX}-pty}"
CHAT_CLAUDE_SERVER_ID="${CHAT_CLAUDE_SERVER_ID:-${SERVER_ID_PREFIX}-chat-claude}"
CHAT_ECHO_SERVER_ID="${CHAT_ECHO_SERVER_ID:-${SERVER_ID_PREFIX}-chat-echo}"
ALLOW_ROOT="${ALLOW_ROOT:-$BUNDLE_ROOT}"
CLAUDE_PATH="${CLAUDE_PATH:-}"
CHAT_PROFILE_FILE="${CHAT_PROFILE_FILE:-}"
START_AGENT="${START_AGENT:-1}"

BIN_DIR="$BUNDLE_ROOT/bin"
UI_DIR="$BUNDLE_ROOT/cc-web"
RUN_DIR="$BUNDLE_ROOT/run"
mkdir -p "$RUN_DIR"

CONTROL_BIN="$BIN_DIR/cc-control"
AGENT_BIN="$BIN_DIR/cc-agent"
CHAT_ECHO_WORKER_BIN="$BIN_DIR/cc-chat-echo"
CHAT_CLAUDE_WORKER_BIN="$BIN_DIR/cc-chat-claude"

CONTROL_STDOUT="$RUN_DIR/cc-control.stdout.log"
CONTROL_STDERR="$RUN_DIR/cc-control.stderr.log"
RESULT_JSON="$RUN_DIR/tokens-and-process.json"

PTY_AGENT_STDOUT="$RUN_DIR/cc-agent-pty.stdout.log"
PTY_AGENT_STDERR="$RUN_DIR/cc-agent-pty.stderr.log"
CLAUDE_AGENT_STDOUT="$RUN_DIR/cc-agent-chat-claude.stdout.log"
CLAUDE_AGENT_STDERR="$RUN_DIR/cc-agent-chat-claude.stderr.log"
ECHO_AGENT_STDOUT="$RUN_DIR/cc-agent-chat-echo.stdout.log"
ECHO_AGENT_STDERR="$RUN_DIR/cc-agent-chat-echo.stderr.log"

BASE_URL="http://127.0.0.1:${CONTROL_PORT}"
CONTROL_PID=""
PTY_AGENT_PID=""
CLAUDE_AGENT_PID=""
ECHO_AGENT_PID=""
RESOLVED_CLAUDE_PATH=""

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing command: $1" >&2
    exit 1
  }
}

detect_claude_path() {
  local preferred="${1:-}"
  if [[ -n "$preferred" ]]; then
    if command -v "$preferred" >/dev/null 2>&1; then
      command -v "$preferred"
      return 0
    fi
    if [[ -x "$preferred" ]]; then
      printf '%s\n' "$preferred"
      return 0
    fi
  fi
  if command -v claude-code >/dev/null 2>&1; then
    command -v claude-code
    return 0
  fi
  if command -v claude >/dev/null 2>&1; then
    command -v claude
    return 0
  fi
  return 1
}

wait_health() {
  local retries=80
  local i
  for ((i=0; i<retries; i++)); do
    if curl -fsS "${BASE_URL}/api/healthz" >/tmp/cc_healthz.$$ 2>/dev/null; then
      if jq -e '.ok == true' /tmp/cc_healthz.$$ >/dev/null 2>&1; then
        rm -f /tmp/cc_healthz.$$
        return 0
      fi
    fi
    sleep 0.5
  done
  rm -f /tmp/cc_healthz.$$
  echo "health check failed: ${BASE_URL}/api/healthz" >&2
  return 1
}

cleanup_on_error() {
  if [[ -n "${ECHO_AGENT_PID}" ]] && kill -0 "$ECHO_AGENT_PID" 2>/dev/null; then
    kill "$ECHO_AGENT_PID" || true
  fi
  if [[ -n "${CLAUDE_AGENT_PID}" ]] && kill -0 "$CLAUDE_AGENT_PID" 2>/dev/null; then
    kill "$CLAUDE_AGENT_PID" || true
  fi
  if [[ -n "${PTY_AGENT_PID}" ]] && kill -0 "$PTY_AGENT_PID" 2>/dev/null; then
    kill "$PTY_AGENT_PID" || true
  fi
  if [[ -n "${CONTROL_PID}" ]] && kill -0 "$CONTROL_PID" 2>/dev/null; then
    kill "$CONTROL_PID" || true
  fi
}
trap cleanup_on_error ERR

need_cmd curl
need_cmd jq

[[ -x "$CONTROL_BIN" ]] || { echo "missing binary: $CONTROL_BIN" >&2; exit 1; }
[[ -d "$UI_DIR" ]] || { echo "missing dir: $UI_DIR" >&2; exit 1; }

if [[ "$START_AGENT" == "1" ]]; then
  [[ -x "$AGENT_BIN" ]] || { echo "missing binary: $AGENT_BIN" >&2; exit 1; }
  [[ -x "$CHAT_ECHO_WORKER_BIN" ]] || { echo "missing binary: $CHAT_ECHO_WORKER_BIN" >&2; exit 1; }
  [[ -x "$CHAT_CLAUDE_WORKER_BIN" ]] || { echo "missing binary: $CHAT_CLAUDE_WORKER_BIN" >&2; exit 1; }
fi

echo "Starting cc-control..."
nohup "$CONTROL_BIN" \
  -addr ":${CONTROL_PORT}" \
  -ui-dir "$UI_DIR" \
  -ui-token= \
  -agent-token= \
  -admin-token "$ADMIN_TOKEN" \
  -audit-path "$RUN_DIR/audit.jsonl" \
  -offline-after-sec 30 \
  >"$CONTROL_STDOUT" 2>"$CONTROL_STDERR" &
CONTROL_PID=$!

wait_health
echo "cc-control is healthy."

echo "Creating tenant token..."
TENANT_RESP="$(curl -fsS -X POST "${BASE_URL}/admin/tokens" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"type":"tenant"}')"

TENANT_TOKEN="$(jq -r '.token // empty' <<<"$TENANT_RESP")"
TENANT_ID="$(jq -r '.tenant_id // empty' <<<"$TENANT_RESP")"
[[ -n "$TENANT_TOKEN" && -n "$TENANT_ID" ]] || {
  echo "failed to create tenant token: $TENANT_RESP" >&2
  exit 1
}

echo "Generating UI + Agent tokens from tenant token..."
ISSUE_RESP="$(curl -fsS -X POST "${BASE_URL}/tenant/tokens" \
  -H "Authorization: Bearer ${TENANT_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"role":"owner"}')"

UI_TOKEN="$(jq -r '.ui.token // empty' <<<"$ISSUE_RESP")"
AGENT_TOKEN="$(jq -r '.agent.token // empty' <<<"$ISSUE_RESP")"
[[ -n "$UI_TOKEN" && -n "$AGENT_TOKEN" ]] || {
  echo "failed to issue ui/agent tokens: $ISSUE_RESP" >&2
  exit 1
}

if [[ "$START_AGENT" == "1" ]]; then
  RESOLVED_CLAUDE_PATH="$(detect_claude_path "$CLAUDE_PATH" || true)"
  if [[ -z "$RESOLVED_CLAUDE_PATH" ]]; then
    echo "cannot find Claude executable. Set CLAUDE_PATH to full path, or ensure 'claude-code'/'claude' is in PATH." >&2
    exit 1
  fi
  if [[ -z "$CHAT_PROFILE_FILE" && -f "$BUNDLE_ROOT/chat-profile.md" ]]; then
    CHAT_PROFILE_FILE="$BUNDLE_ROOT/chat-profile.md"
  fi
  if [[ -n "$CHAT_PROFILE_FILE" && ! -f "$CHAT_PROFILE_FILE" ]]; then
    echo "chat profile file not found: $CHAT_PROFILE_FILE" >&2
    exit 1
  fi

  echo "Claude path: $RESOLVED_CLAUDE_PATH"
  if [[ -n "$CHAT_PROFILE_FILE" ]]; then
    echo "Chat profile file: $CHAT_PROFILE_FILE"
  fi

  echo "Starting cc-agent #1 PTY (${PTY_SERVER_ID})..."
  nohup "$AGENT_BIN" \
    -control-url "ws://127.0.0.1:${CONTROL_PORT}/ws/agent" \
    -agent-token "$AGENT_TOKEN" \
    -server-id "$PTY_SERVER_ID" \
    -allow-root "$ALLOW_ROOT" \
    -claude-path "$RESOLVED_CLAUDE_PATH" \
    >"$PTY_AGENT_STDOUT" 2>"$PTY_AGENT_STDERR" &
  PTY_AGENT_PID=$!

  echo "Starting cc-agent #2 chat-claude (${CHAT_CLAUDE_SERVER_ID})..."
  env \
    CC_CLAUDE_CMD="$RESOLVED_CLAUDE_PATH" \
    CC_CLAUDE_PROFILE_FILE="$CHAT_PROFILE_FILE" \
    nohup "$AGENT_BIN" \
      -control-url "ws://127.0.0.1:${CONTROL_PORT}/ws/agent" \
      -agent-token "$AGENT_TOKEN" \
      -server-id "$CHAT_CLAUDE_SERVER_ID" \
      -allow-root "$ALLOW_ROOT" \
      -claude-path "$RESOLVED_CLAUDE_PATH" \
      -chat-worker "$CHAT_CLAUDE_WORKER_BIN" \
      >"$CLAUDE_AGENT_STDOUT" 2>"$CLAUDE_AGENT_STDERR" &
  CLAUDE_AGENT_PID=$!

  echo "Starting cc-agent #3 chat-echo (${CHAT_ECHO_SERVER_ID})..."
  nohup "$AGENT_BIN" \
    -control-url "ws://127.0.0.1:${CONTROL_PORT}/ws/agent" \
    -agent-token "$AGENT_TOKEN" \
    -server-id "$CHAT_ECHO_SERVER_ID" \
    -allow-root "$ALLOW_ROOT" \
    -claude-path "$RESOLVED_CLAUDE_PATH" \
    -chat-worker "$CHAT_ECHO_WORKER_BIN" \
    >"$ECHO_AGENT_STDOUT" 2>"$ECHO_AGENT_STDERR" &
  ECHO_AGENT_PID=$!
fi

jq -n \
  --arg base_url "$BASE_URL" \
  --arg admin_token "$ADMIN_TOKEN" \
  --arg tenant_id "$TENANT_ID" \
  --arg tenant_token "$TENANT_TOKEN" \
  --arg ui_token "$UI_TOKEN" \
  --arg agent_token "$AGENT_TOKEN" \
  --arg control_pid "${CONTROL_PID:-}" \
  --arg control_stdout "$CONTROL_STDOUT" \
  --arg control_stderr "$CONTROL_STDERR" \
  --arg pty_server_id "$PTY_SERVER_ID" \
  --arg pty_pid "${PTY_AGENT_PID:-}" \
  --arg pty_stdout "$PTY_AGENT_STDOUT" \
  --arg pty_stderr "$PTY_AGENT_STDERR" \
  --arg claude_server_id "$CHAT_CLAUDE_SERVER_ID" \
  --arg claude_pid "${CLAUDE_AGENT_PID:-}" \
  --arg claude_stdout "$CLAUDE_AGENT_STDOUT" \
  --arg claude_stderr "$CLAUDE_AGENT_STDERR" \
  --arg echo_server_id "$CHAT_ECHO_SERVER_ID" \
  --arg echo_pid "${ECHO_AGENT_PID:-}" \
  --arg echo_stdout "$ECHO_AGENT_STDOUT" \
  --arg echo_stderr "$ECHO_AGENT_STDERR" \
  --arg resolved_claude_path "${RESOLVED_CLAUDE_PATH:-}" \
  --arg chat_profile_file "${CHAT_PROFILE_FILE:-}" \
  --arg web_terminal "${BASE_URL}/" \
  --arg web_chat "${BASE_URL}/chat" \
  --arg admin "${BASE_URL}/admin" \
  --arg tenant "${BASE_URL}/tenant" \
  '{
    base_url: $base_url,
    admin_token: $admin_token,
    tenant_id: $tenant_id,
    tenant_token: $tenant_token,
    ui_token: $ui_token,
    agent_token: $agent_token,
    control: {
      pid: (if $control_pid == "" then null else ($control_pid|tonumber) end),
      logs: {
        stdout: $control_stdout,
        stderr: $control_stderr
      }
    },
    agents: {
      pty: {
        server_id: $pty_server_id,
        pid: (if $pty_pid == "" then null else ($pty_pid|tonumber) end),
        logs: { stdout: $pty_stdout, stderr: $pty_stderr }
      },
      chat_claude: {
        server_id: $claude_server_id,
        pid: (if $claude_pid == "" then null else ($claude_pid|tonumber) end),
        logs: { stdout: $claude_stdout, stderr: $claude_stderr }
      },
      chat_echo: {
        server_id: $echo_server_id,
        pid: (if $echo_pid == "" then null else ($echo_pid|tonumber) end),
        logs: { stdout: $echo_stdout, stderr: $echo_stderr }
      }
    },
    resolved_claude_path: (if $resolved_claude_path == "" then null else $resolved_claude_path end),
    chat_profile_file: (if $chat_profile_file == "" then null else $chat_profile_file end),
    urls: {
      web_terminal: $web_terminal,
      web_chat: $web_chat,
      admin: $admin,
      tenant: $tenant
    }
  }' >"$RESULT_JSON"

echo
echo "Done."
echo "Tenant ID:    $TENANT_ID"
echo "Tenant Token: $TENANT_TOKEN"
echo "UI Token:     $UI_TOKEN"
echo "Agent Token:  $AGENT_TOKEN"
echo
echo "Server IDs:"
echo "  PTY:         $PTY_SERVER_ID"
echo "  chat-claude: $CHAT_CLAUDE_SERVER_ID"
echo "  chat-echo:   $CHAT_ECHO_SERVER_ID"
echo
echo "Result file:  $RESULT_JSON"
echo "Chat URL:     ${BASE_URL}/chat"
echo
echo "Stop commands:"
if [[ -n "$ECHO_AGENT_PID" ]]; then
  echo "  kill ${ECHO_AGENT_PID}"
fi
if [[ -n "$CLAUDE_AGENT_PID" ]]; then
  echo "  kill ${CLAUDE_AGENT_PID}"
fi
if [[ -n "$PTY_AGENT_PID" ]]; then
  echo "  kill ${PTY_AGENT_PID}"
fi
echo "  kill ${CONTROL_PID}"
