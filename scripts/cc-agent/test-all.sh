#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT_DIR="$ROOT_DIR/scripts/cc-agent"

echo "[cc-agent][all] 1/3 unit tests"
bash "$SCRIPT_DIR/test-unit.sh"

echo "[cc-agent][all] 2/3 integration tests"
bash "$SCRIPT_DIR/test-integration-local.sh"

if [[ "${RUN_E2E_APPROVE:-0}" == "1" ]]; then
  echo "[cc-agent][all] 3/3 e2e approve tests"
  bash "$SCRIPT_DIR/test-e2e-approve-click-fix-case.sh"
else
  echo "[cc-agent][all] 3/3 e2e approve tests (skipped: set RUN_E2E_APPROVE=1)"
fi

echo "[cc-agent][all] PASS"
