#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT_DIR="$ROOT_DIR/scripts/cc-agent"

echo "[cc-agent][all] 1/2 unit tests"
bash "$SCRIPT_DIR/test-unit.sh"

echo "[cc-agent][all] 2/2 integration tests"
bash "$SCRIPT_DIR/test-integration-local.sh"

echo "[cc-agent][all] PASS"
