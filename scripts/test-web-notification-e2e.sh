#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if ! command -v npx >/dev/null 2>&1; then
  echo "npx not found in PATH" >&2
  exit 1
fi

cd "$ROOT_DIR"

export CC_WEB_E2E_CLAUDE_MODE="${CC_WEB_E2E_CLAUDE_MODE:-fake}"
export CC_WEB_E2E_SCREENSHOT="${CC_WEB_E2E_SCREENSHOT:-on}"
export CC_WEB_E2E_PORT="${CC_WEB_E2E_PORT:-18114}"

echo "[web-notification-e2e] running notification smoke test..."
npx playwright test \
  --config=tests/web-e2e/playwright.config.mjs \
  tests/web-e2e/specs/notification.spec.js

echo "[web-notification-e2e] done. screenshots/artifacts are under test-results/"
