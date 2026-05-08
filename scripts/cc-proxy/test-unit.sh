#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

if ! command -v go >/dev/null 2>&1; then
  echo "go not found in PATH" >&2
  exit 1
fi

echo "[cc-proxy][unit] running go test ..."
go -C "$ROOT_DIR/cc-proxy" test ./... -count=1 -v
echo "[cc-proxy][unit] PASS"
