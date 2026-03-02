#!/usr/bin/env bash
# agent-control cc-agent installer
# Usage: curl -fsSL https://cc-remote.app/install.sh | bash
#    or: curl -fsSL https://raw.githubusercontent.com/xuzhougeng/agent-control/main/scripts/install.sh | bash
set -euo pipefail

GITHUB_REPO="${GITHUB_REPO:-xuzhougeng/agent-control}"
VERSION="${VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
NEED_SUDO=""

detect_os_arch() {
  local os arch
  case "$(uname -s)" in
    Linux)   os=linux ;;
    Darwin)  os=darwin ;;
    *)
      echo "Unsupported OS: $(uname -s). On Windows use PowerShell:" >&2
      echo "  irm https://cc-remote.app/install.ps1 | iex" >&2
      exit 1
      ;;
  esac
  case "$(uname -m)" in
    x86_64|amd64) arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    *)
      echo "Unsupported arch: $(uname -m)" >&2
      exit 1
      ;;
  esac
  echo "${os}-${arch}"
}

need_sudo() {
  [[ -w "$INSTALL_DIR" ]] || NEED_SUDO=sudo
}

download_url() {
  local binary="$1" platform="$2"
  if [[ "$VERSION" == "latest" ]]; then
    echo "https://github.com/${GITHUB_REPO}/releases/latest/download/${binary}-${platform}"
  else
    echo "https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/${binary}-${platform}"
  fi
}

install_binary() {
  local binary="$1" platform="$2"
  local url dest
  url=$(download_url "$binary" "$platform")
  dest="${tmp}/${binary}"
  echo "==> Downloading $url"
  if ! curl -fsSL -o "$dest" "$url"; then
    echo "Download failed. Ensure a release exists with asset: ${binary}-${platform}" >&2
    echo "See: https://github.com/${GITHUB_REPO}/releases" >&2
    exit 1
  fi
  chmod +x "$dest"
  $NEED_SUDO mv "$dest" "${INSTALL_DIR}/${binary}"
  echo "==> Installed: ${INSTALL_DIR}/${binary}"
}

main() {
  echo "==> Agent Control installer"
  platform=$(detect_os_arch)
  echo "==> Platform: $platform"

  need_sudo
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT

  install_binary "cc-agent" "$platform"
  install_binary "cc-chat-claude" "$platform"

  echo ""
  echo "Next: get an agent token from your control plane, then run:"
  echo "  cc-agent -control-url wss://YOUR_CONTROL/ws/agent -agent-token YOUR_TOKEN -server-id srv-01 -allow-root /path/to/cwd -claude-path /path/to/claude"
  echo ""
  echo "Docs: https://github.com/${GITHUB_REPO}/blob/main/docs/getting-started.md"
}

main "$@"
