#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_BASE="${OUT_BASE:-$REPO_ROOT/dist}"
RELEASE_NAME="${RELEASE_NAME:-multichat-release}"
AGENT_TARGETS="${AGENT_TARGETS:-linux/amd64,linux/arm64,darwin/amd64,darwin/arm64,windows/amd64,windows/arm64}"

usage() {
  cat <<'EOF'
Usage:
  bash scripts/build-release-bundle.sh [--out-base dist] [--name multichat-release] [--agent-targets "linux/amd64,windows/amd64"]

Defaults:
  cc-control target: linux/amd64 (fixed)
  agent targets: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64

Examples:
  bash scripts/build-release-bundle.sh
  bash scripts/build-release-bundle.sh --agent-targets "linux/amd64,windows/amd64"
  OUT_BASE=/tmp/dist RELEASE_NAME=v1.0.0 bash scripts/build-release-bundle.sh
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --out-base)
      OUT_BASE="$2"
      shift 2
      ;;
    --name)
      RELEASE_NAME="$2"
      shift 2
      ;;
    --agent-targets)
      AGENT_TARGETS="$2"
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

release_dir="$OUT_BASE/$RELEASE_NAME"
artifacts_dir="$release_dir/artifacts"
server_root="$release_dir/server"
client_root="$release_dir/client"
control_dir="$server_root/bin"

VERSION="${VERSION:-}"
if [[ -z "$VERSION" ]]; then
  if [[ "$RELEASE_NAME" == v[0-9]* ]]; then
    VERSION="$RELEASE_NAME"
  else
    VERSION="$(git -C "$REPO_ROOT" describe --tags --dirty --always 2>/dev/null || echo dev)"
  fi
fi
COMMIT="${COMMIT:-$(git -C "$REPO_ROOT" rev-parse HEAD 2>/dev/null || true)}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
CC_AGENT_LDFLAGS="-s -w -X cc-agent/internal/version.Version=${VERSION} -X cc-agent/internal/version.Commit=${COMMIT} -X cc-agent/internal/version.Date=${BUILD_DATE}"

mkdir -p "$OUT_BASE"
rm -rf "$release_dir"
mkdir -p "$artifacts_dir" "$control_dir" "$client_root"

echo "==> release directory: $release_dir"
echo "==> cc-agent version: $VERSION"

echo "==> building server (cc-control linux/amd64 + cc-web)"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go -C "$REPO_ROOT/cc-control" build -o "$control_dir/cc-control" ./cmd/cc-control
cp -R "$REPO_ROOT/cc-web" "$server_root/cc-web"

build_agent_target() {
  local target="$1"
  local goos="${target%%/*}"
  local goarch="${target##*/}"
  if [[ -z "$goos" || -z "$goarch" || "$goos" == "$goarch" ]]; then
    echo "invalid target: $target (expected os/arch)" >&2
    exit 1
  fi

  local platform="${goos}-${goarch}"
  local suffix=""
  if [[ "$goos" == "windows" ]]; then
    suffix=".exe"
  fi

  local out_dir="$client_root/$platform"
  mkdir -p "$out_dir/bin"

  echo "==> building agent binaries for $goos/$goarch"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go -C "$REPO_ROOT/cc-proxy" build -ldflags="-s -w" -o "$out_dir/bin/cc-proxy${suffix}" ./cmd/cc-proxy
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go -C "$REPO_ROOT/cc-proxy" build -ldflags="-s -w" -o "$out_dir/bin/cc-chat-claude${suffix}" ./cmd/cc-chat-claude
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go -C "$REPO_ROOT/cc-proxy" build -ldflags="-s -w" -o "$out_dir/bin/cc-chat-echo${suffix}" ./cmd/cc-chat-echo
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go -C "$REPO_ROOT/cc-agent" build -ldflags="$CC_AGENT_LDFLAGS" -o "$out_dir/bin/cc-agent${suffix}" ./cmd/cc-agent

  cp "$REPO_ROOT/scripts/chat-profile.example.md" "$out_dir/chat-profile.example.md"
  cp "$REPO_ROOT/scripts/chat-profile.example.md" "$out_dir/chat-profile.md"

  if [[ "$goos" == "windows" ]]; then
    cp "$REPO_ROOT/scripts/windows/run-multichat-win.ps1" "$out_dir/run-multichat-win.ps1"
    cp "$REPO_ROOT/scripts/windows/stop-multichat-win.ps1" "$out_dir/stop-multichat-win.ps1"
  else
    cp "$REPO_ROOT/scripts/linux/run-multichat-linux.sh" "$out_dir/run-multichat.sh"
    cp "$REPO_ROOT/scripts/linux/stop-multichat-linux.sh" "$out_dir/stop-multichat.sh"
    chmod +x "$out_dir/run-multichat.sh" "$out_dir/stop-multichat.sh"
  fi
}

IFS=',' read -r -a target_list <<<"$AGENT_TARGETS"
for target in "${target_list[@]}"; do
  target="$(echo "$target" | xargs)"
  [[ -z "$target" ]] && continue
  build_agent_target "$target"
done

cat >"$release_dir/README.txt" <<EOF
${RELEASE_NAME}
================

Output packages:
- artifacts/server.tar.gz
- artifacts/client.tar.gz

server.tar.gz includes:
- server/bin/cc-control (linux/amd64)
- server/cc-web/

client.tar.gz includes:
- client/<os-arch>/bin/cc-proxy[.exe]         (legacy PTY wrapper for Claude/Codex/Gemini CLIs)
- client/<os-arch>/bin/cc-chat-claude[.exe]   (Claude headless chat worker)
- client/<os-arch>/bin/cc-chat-echo[.exe]     (echo chat worker for testing)
- client/<os-arch>/bin/cc-agent[.exe]         (standalone server-ops agent, new in v0.7.0)
- per-platform run/stop scripts and chat profile templates

Build inputs:
- AGENT_TARGETS=${AGENT_TARGETS}
EOF

echo "==> packaging server.tar.gz"
tar -C "$release_dir" -czf "$artifacts_dir/server.tar.gz" "server"
echo "==> packaging client.tar.gz"
tar -C "$release_dir" -czf "$artifacts_dir/client.tar.gz" "client"

echo
echo "Release bundle ready: $release_dir"
echo "Artifacts ready in: $artifacts_dir"
