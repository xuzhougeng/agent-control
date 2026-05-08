#!/usr/bin/env bash
# agent-control cc-proxy installer
# Usage: curl -fsSL https://cc-remote.app/install.sh | bash
#    or: curl -fsSL https://raw.githubusercontent.com/xuzhougeng/agent-control/main/scripts/install.sh | bash
set -euo pipefail

GITHUB_REPO="${GITHUB_REPO:-xuzhougeng/agent-control}"
VERSION="${VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

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
  mv "$dest" "${INSTALL_DIR}/${binary}"
  echo "==> Installed: ${INSTALL_DIR}/${binary}"
}

ensure_install_dir() {
  mkdir -p "$INSTALL_DIR"
  if [[ ! -w "$INSTALL_DIR" ]]; then
    echo "Install directory is not writable: $INSTALL_DIR" >&2
    echo "Try setting INSTALL_DIR to a writable path, for example:" >&2
    echo "  INSTALL_DIR=\"\$HOME/.local/bin\" bash install.sh" >&2
    exit 1
  fi
}

normalize_control_url() {
  local input url
  input="$(printf '%s' "$1" | xargs)"
  while [[ "$input" == */ ]]; do
    input="${input%/}"
  done

  if [[ "$input" == ws://* || "$input" == wss://* ]]; then
    url="$input"
  elif [[ "$input" == https://* ]]; then
    url="wss://${input#https://}"
  elif [[ "$input" == http://* ]]; then
    url="ws://${input#http://}"
  else
    url="wss://${input}"
  fi

  if [[ "$url" != */ws/agent ]]; then
    url="${url}/ws/agent"
  fi
  echo "$url"
}

extract_host_from_url() {
  local url no_scheme host
  url="$1"
  no_scheme="${url#*://}"
  no_scheme="${no_scheme%%/*}"

  if [[ "$no_scheme" == \[*\]* ]]; then
    host="${no_scheme#\[}"
    host="${host%%]*}"
  else
    host="${no_scheme%%:*}"
  fi
  echo "$host"
}

is_ipv4_address() {
  local ip octet
  ip="$1"
  if [[ ! "$ip" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; then
    return 1
  fi
  IFS='.' read -r -a octets <<< "$ip"
  for octet in "${octets[@]}"; do
    if ! [[ "$octet" =~ ^[0-9]+$ ]] || ((octet < 0 || octet > 255)); then
      return 1
    fi
  done
  return 0
}

is_ipv6_address() {
  local ip
  ip="$1"
  [[ "$ip" == *:* ]] && [[ "$ip" =~ ^[0-9A-Fa-f:]+$ ]]
}

is_ip_address() {
  local host
  host="$1"
  is_ipv4_address "$host" || is_ipv6_address "$host"
}

detect_tls_skip_verify() {
  local host
  if [[ -n "${TLS_SKIP_VERIFY:-}" ]]; then
    case "${TLS_SKIP_VERIFY,,}" in
      1|true|yes|y) USE_TLS_SKIP_VERIFY=1 ;;
      *) USE_TLS_SKIP_VERIFY=0 ;;
    esac
    return
  fi

  host="$(extract_host_from_url "$CONTROL_URL")"
  if is_ip_address "$host"; then
    USE_TLS_SKIP_VERIFY=1
    echo "==> Control URL host is IP (${host}), enabling -tls-skip-verify"
  else
    USE_TLS_SKIP_VERIFY=0
  fi
}

detect_current_shell() {
  local shell_path shell_name
  shell_path="${SHELL:-}"
  if [[ -n "$shell_path" ]]; then
    shell_name="$(basename "$shell_path")"
  else
    shell_name="unknown"
  fi
  echo "$shell_name"
}

print_path_setup_examples() {
  local shell_name
  shell_name="$(detect_current_shell)"
  echo "PATH does not include ${INSTALL_DIR} for new shells."
  echo "==> Current SHELL: ${SHELL:-unknown} (${shell_name})"
  echo "Add it permanently with one of these:"
  echo "  bash: echo 'export PATH=\"${INSTALL_DIR}:\$PATH\"' >> ~/.bashrc && source ~/.bashrc"
  echo "  zsh:  echo 'export PATH=\"${INSTALL_DIR}:\$PATH\"' >> ~/.zshrc && source ~/.zshrc"
  echo "  fish: fish_add_path \"${INSTALL_DIR}\""
  echo "  sh:   echo 'export PATH=\"${INSTALL_DIR}:\$PATH\"' >> ~/.profile"
}

read_prompt() {
  local prompt="$1"
  local __var_name="$2"
  local __value=""
  if [[ -r /dev/tty ]]; then
    read -r -p "$prompt" __value < /dev/tty
  else
    read -r -p "$prompt" __value
  fi
  printf -v "$__var_name" '%s' "$__value"
}

read_secret_prompt() {
  local prompt="$1"
  local __var_name="$2"
  local __value=""
  if [[ -r /dev/tty ]]; then
    read -r -s -p "$prompt" __value < /dev/tty
    printf '\n' > /dev/tty
  else
    read -r -s -p "$prompt" __value
    echo ""
  fi
  printf -v "$__var_name" '%s' "$__value"
}

resolve_claude_path() {
  local raw line alias_target
  raw="$(which claude 2>/dev/null || true)"
  if [[ -z "$raw" ]]; then
    return 1
  fi

  while IFS= read -r line; do
    [[ -z "$line" ]] && continue

    if [[ "$line" == *"aliased to "* ]]; then
      alias_target="${line#*aliased to }"
      alias_target="${alias_target%\'}"
      alias_target="${alias_target#\'}"
      # -claude-path only supports a binary path; aliases with args are skipped.
      if [[ "$alias_target" == *" "* ]]; then
        continue
      fi
      if [[ "$alias_target" == /* && -x "$alias_target" ]]; then
        echo "$alias_target"
        return 0
      fi
      if command -v "$alias_target" >/dev/null 2>&1; then
        command -v "$alias_target"
        return 0
      fi
      continue
    fi

    if [[ "$line" == "claude is "* ]]; then
      line="${line#claude is }"
    fi
    if [[ "$line" == /* && -x "$line" ]]; then
      echo "$line"
      return 0
    fi
  done <<< "$raw"

  if command -v claude >/dev/null 2>&1; then
    command -v claude
    return 0
  fi
  return 1
}

prompt_control_url() {
  local input
  if [[ -n "${CONTROL_URL:-}" ]]; then
    CONTROL_URL="$(normalize_control_url "$CONTROL_URL")"
    return
  fi
  while true; do
    read_prompt "Control URL (e.g. https://cc-remote.app): " input
    if [[ -n "$(printf '%s' "$input" | xargs)" ]]; then
      CONTROL_URL="$(normalize_control_url "$input")"
      return
    fi
    echo "Control URL cannot be empty."
  done
}

prompt_agent_token() {
  if [[ -n "${AGENT_TOKEN:-}" ]]; then
    return
  fi
  while true; do
    read_secret_prompt "Agent Token: " AGENT_TOKEN
    if [[ -n "$(printf '%s' "$AGENT_TOKEN" | xargs)" ]]; then
      return
    fi
    echo "Agent Token cannot be empty."
  done
}

print_run_command() {
  local cmd
  cmd="cc-proxy -control-url \"$CONTROL_URL\" -agent-token <YOUR_TOKEN> -server-id \"$SERVER_ID\" -allow-root \"$ALLOW_ROOT\""
  if [[ -n "${CLAUDE_PATH:-}" ]]; then
    cmd="$cmd -claude-path \"$CLAUDE_PATH\""
  fi
  if [[ "${USE_TLS_SKIP_VERIFY:-0}" -eq 1 ]]; then
    cmd="$cmd -tls-skip-verify"
  fi
  echo "$cmd"
}

main() {
  echo "==> Agent Control installer"
  platform=$(detect_os_arch)
  echo "==> Platform: $platform"

  ensure_install_dir
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT

  install_binary "cc-proxy" "$platform"
  install_binary "cc-chat-claude" "$platform"

  prompt_control_url
  prompt_agent_token
  detect_tls_skip_verify

  if CLAUDE_PATH="$(resolve_claude_path)"; then
    echo "==> Claude detected: $CLAUDE_PATH"
  else
    CLAUDE_PATH=""
    echo "==> Claude not found via 'which claude'."
    echo "    Install Claude CLI, or run later with -claude-path /path/to/claude."
  fi

  SERVER_ID="${SERVER_ID:-srv-$(hostname | tr '[:upper:]' '[:lower:]' | tr -cs 'a-z0-9' '-')}"
  ALLOW_ROOT="${ALLOW_ROOT:-$PWD}"
  had_install_dir_in_path=0
  if [[ ":$PATH:" == *":${INSTALL_DIR}:"* ]]; then
    had_install_dir_in_path=1
  fi
  export PATH="${INSTALL_DIR}:$PATH"

  echo ""
  if [[ "$had_install_dir_in_path" -eq 0 ]]; then
    print_path_setup_examples
    echo ""
  fi
  echo "Reusable command:"
  echo "  $(print_run_command)"
  echo ""
  echo "Docs: https://github.com/${GITHUB_REPO}/blob/main/docs/getting-started.md"
  echo ""
  echo "==> Launching cc-proxy..."

  if [[ -n "$CLAUDE_PATH" ]]; then
    if [[ "${USE_TLS_SKIP_VERIFY:-0}" -eq 1 ]]; then
      exec "${INSTALL_DIR}/cc-proxy" \
        -control-url "$CONTROL_URL" \
        -agent-token "$AGENT_TOKEN" \
        -server-id "$SERVER_ID" \
        -allow-root "$ALLOW_ROOT" \
        -claude-path "$CLAUDE_PATH" \
        -tls-skip-verify
    else
      exec "${INSTALL_DIR}/cc-proxy" \
        -control-url "$CONTROL_URL" \
        -agent-token "$AGENT_TOKEN" \
        -server-id "$SERVER_ID" \
        -allow-root "$ALLOW_ROOT" \
        -claude-path "$CLAUDE_PATH"
    fi
  else
    if [[ "${USE_TLS_SKIP_VERIFY:-0}" -eq 1 ]]; then
      exec "${INSTALL_DIR}/cc-proxy" \
        -control-url "$CONTROL_URL" \
        -agent-token "$AGENT_TOKEN" \
        -server-id "$SERVER_ID" \
        -allow-root "$ALLOW_ROOT" \
        -tls-skip-verify
    else
      exec "${INSTALL_DIR}/cc-proxy" \
        -control-url "$CONTROL_URL" \
        -agent-token "$AGENT_TOKEN" \
        -server-id "$SERVER_ID" \
        -allow-root "$ALLOW_ROOT"
    fi
  fi
}

main "$@"
