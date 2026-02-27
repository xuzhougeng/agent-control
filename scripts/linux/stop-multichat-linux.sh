#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -d "$SCRIPT_DIR/run" ]]; then
  BUNDLE_ROOT="$SCRIPT_DIR"
else
  BUNDLE_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

RESULT_JSON="${1:-${RESULT_JSON:-$BUNDLE_ROOT/run/tokens-and-process.json}}"

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing command: $1" >&2
    exit 1
  }
}

need_cmd jq

if [[ ! -f "$RESULT_JSON" ]]; then
  echo "result file not found: $RESULT_JSON" >&2
  exit 1
fi

stop_pid() {
  local pid="$1"
  local name="$2"
  if [[ -z "$pid" || "$pid" == "null" ]]; then
    return 0
  fi
  if ! [[ "$pid" =~ ^[0-9]+$ ]]; then
    return 0
  fi
  if kill -0 "$pid" 2>/dev/null; then
    echo "stopping ${name} (pid=${pid})"
    kill "$pid" 2>/dev/null || true
    sleep 1
    if kill -0 "$pid" 2>/dev/null; then
      echo "force stopping ${name} (pid=${pid})"
      kill -9 "$pid" 2>/dev/null || true
    fi
  else
    echo "${name} already stopped (pid=${pid})"
  fi
}

PTY_PID="$(jq -r '.agents.pty.pid // empty' "$RESULT_JSON")"
CLAUDE_PID="$(jq -r '.agents.chat_claude.pid // empty' "$RESULT_JSON")"
ECHO_PID="$(jq -r '.agents.chat_echo.pid // empty' "$RESULT_JSON")"
CONTROL_PID="$(jq -r '.control.pid // empty' "$RESULT_JSON")"

# Backward compatibility with old format.
if [[ -z "$CONTROL_PID" ]]; then
  CONTROL_PID="$(jq -r '.control_pid // empty' "$RESULT_JSON")"
fi
if [[ -z "$ECHO_PID" ]]; then
  ECHO_PID="$(jq -r '.agent_pid // empty' "$RESULT_JSON")"
fi

stop_pid "$ECHO_PID" "agent-chat-echo"
stop_pid "$CLAUDE_PID" "agent-chat-claude"
stop_pid "$PTY_PID" "agent-pty"
stop_pid "$CONTROL_PID" "cc-control"

echo "stop completed."
