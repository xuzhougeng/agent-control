#!/usr/bin/env bash
set -euo pipefail

# Minimal E2E smoke test for chat sessions using Claude Code headless worker.
# Requires: claude CLI authenticated in this shell.
# Usage: bash scripts/test-chat-claude-e2e.sh

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

if ! command -v claude >/dev/null 2>&1; then
  echo "claude CLI not found in PATH" >&2
  exit 1
fi

if ! claude auth status | grep -q '"loggedIn": true'; then
  echo "claude CLI not logged in; run 'claude auth login' first" >&2
  exit 1
fi

echo "==> Building binaries..."
(cd "$ROOT_DIR" && go build -o /tmp/cc-control ./cc-control/cmd/cc-control)
(cd "$ROOT_DIR" && go build -o /tmp/cc-agent ./cc-agent/cmd/cc-agent)
(cd "$ROOT_DIR" && go build -o /tmp/cc-chat-claude ./cc-agent/cmd/cc-chat-claude)

AUDIT_PATH=$(mktemp /tmp/audit-XXXXXX.jsonl)
CONTROL_PID=""
AGENT_PID=""
cleanup() {
  [ -n "$CONTROL_PID" ] && kill "$CONTROL_PID" 2>/dev/null || true
  [ -n "$AGENT_PID" ] && kill "$AGENT_PID" 2>/dev/null || true
  rm -f "$AUDIT_PATH"
}
trap cleanup EXIT

echo "==> Starting cc-control..."
/tmp/cc-control \
  -addr :18090 \
  -admin-token admin-dev-token \
  -ui-dir "$ROOT_DIR/cc-web" \
  -audit-path "$AUDIT_PATH" &
CONTROL_PID=$!
sleep 1

echo "==> Starting cc-agent with Claude chat worker..."
/tmp/cc-agent \
  -control-url ws://127.0.0.1:18090/ws/agent \
  -server-id srv-test \
  -agent-token agent-dev-token \
  -allow-root /tmp \
  -chat-worker /tmp/cc-chat-claude &
AGENT_PID=$!
sleep 1

TOKEN="admin-dev-token"
BASE="http://127.0.0.1:18090"

echo "==> Creating chat session..."
SESSION=$(curl -sf -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"server_id":"srv-test","session_type":"chat","cwd":"/tmp","env":{"CC_CLAUDE_ALLOWED_TOOLS":""}}' \
  "$BASE/api/sessions")
SESSION_ID=$(echo "$SESSION" | python3 -c "import sys,json; print(json.load(sys.stdin)['session_id'])")

echo "==> Open /chat and send messages to verify stateful memory."
echo "Session ID: $SESSION_ID"

echo "==> Chat E2E ready (manual verification)."
