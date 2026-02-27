#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_BASE="${OUT_BASE:-$REPO_ROOT/dist}"
TARGETS="${TARGETS:-linux,windows}"
ARCH="${ARCH:-amd64}"

usage() {
  cat <<'EOF'
Usage:
  bash scripts/build-multichat-test-bundle.sh [--targets linux,windows] [--arch amd64] [--out-base dist]

Examples:
  bash scripts/build-multichat-test-bundle.sh
  bash scripts/build-multichat-test-bundle.sh --targets linux
  TARGETS=windows ARCH=amd64 bash scripts/build-multichat-test-bundle.sh
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --targets)
      TARGETS="$2"
      shift 2
      ;;
    --arch)
      ARCH="$2"
      shift 2
      ;;
    --out-base)
      OUT_BASE="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown arg: $1" >&2
      usage
      exit 1
      ;;
  esac
done

IFS=',' read -r -a TARGET_LIST <<<"$TARGETS"
mkdir -p "$OUT_BASE"

build_one() {
  local goos="$1"
  local suffix=""
  if [[ "$goos" == "windows" ]]; then
    suffix=".exe"
  fi

  local bundle_dir="$OUT_BASE/multichat-test-${goos}-${ARCH}"
  local bin_dir="$bundle_dir/bin"
  local web_dir="$bundle_dir/cc-web"

  echo "==> preparing ${bundle_dir}"
  rm -rf "$bundle_dir"
  mkdir -p "$bin_dir" "$web_dir"

  echo "==> building binaries for ${goos}/${ARCH}"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$ARCH" go -C "$REPO_ROOT/cc-control" build -o "$bin_dir/cc-control${suffix}" ./cmd/cc-control
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$ARCH" go -C "$REPO_ROOT/cc-agent" build -o "$bin_dir/cc-agent${suffix}" ./cmd/cc-agent
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$ARCH" go -C "$REPO_ROOT/cc-agent" build -o "$bin_dir/cc-chat-echo${suffix}" ./cmd/cc-chat-echo
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$ARCH" go -C "$REPO_ROOT/cc-agent" build -o "$bin_dir/cc-chat-claude${suffix}" ./cmd/cc-chat-claude

  echo "==> copying web assets"
  cp -R "$REPO_ROOT/cc-web/." "$web_dir/"

  if [[ "$goos" == "windows" ]]; then
    cp "$REPO_ROOT/scripts/windows/run-multichat-win.ps1" "$bundle_dir/run-multichat-win.ps1"
    cp "$REPO_ROOT/scripts/windows/stop-multichat-win.ps1" "$bundle_dir/stop-multichat-win.ps1"
    cat >"$bundle_dir/README.txt" <<'EOF'
Windows Multi-Chat Test Bundle
==============================

Run (PowerShell):
  powershell -ExecutionPolicy Bypass -File .\run-multichat-win.ps1
Stop (PowerShell):
  powershell -ExecutionPolicy Bypass -File .\stop-multichat-win.ps1

Output:
  .\run\tokens-and-process.json

This starts 1 control-plane + 3 agents:
- <prefix>-pty
- <prefix>-chat-claude
- <prefix>-chat-echo
EOF
  else
    cp "$REPO_ROOT/scripts/linux/run-multichat-linux.sh" "$bundle_dir/run-multichat.sh"
    cp "$REPO_ROOT/scripts/linux/stop-multichat-linux.sh" "$bundle_dir/stop-multichat.sh"
    chmod +x "$bundle_dir/run-multichat.sh"
    chmod +x "$bundle_dir/stop-multichat.sh"
    cat >"$bundle_dir/README.txt" <<'EOF'
Linux Multi-Chat Test Bundle
============================

Run:
  bash ./run-multichat.sh
Stop:
  bash ./stop-multichat.sh

Output:
  ./run/tokens-and-process.json

This starts 1 control-plane + 3 agents:
- <prefix>-pty
- <prefix>-chat-claude
- <prefix>-chat-echo
EOF
  fi

  local tarball="$OUT_BASE/multichat-test-${goos}-${ARCH}.tar.gz"
  echo "==> packaging ${tarball}"
  tar -C "$OUT_BASE" -czf "$tarball" "$(basename "$bundle_dir")"

  if [[ "$goos" == "windows" ]] && command -v zip >/dev/null 2>&1; then
    local zipfile="$OUT_BASE/multichat-test-${goos}-${ARCH}.zip"
    echo "==> packaging ${zipfile}"
    (
      cd "$OUT_BASE"
      rm -f "$zipfile"
      zip -rq "$zipfile" "$(basename "$bundle_dir")"
    )
  fi
}

for t in "${TARGET_LIST[@]}"; do
  case "$t" in
    linux|windows)
      build_one "$t"
      ;;
    *)
      echo "unsupported target: $t (allowed: linux, windows)" >&2
      exit 1
      ;;
  esac
done

echo
echo "Bundles ready in: $OUT_BASE"
