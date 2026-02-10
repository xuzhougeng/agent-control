# Agent Control (Claude Code Control Plane MVP)

Minimal multi-server controller for launching and managing `claude-code` sessions.

## Layout

- `cc-control/`: control plane (REST + WS + audit + prompt detection)
- `cc-agent/`: per-server agent (WS outbound, PTY spawn/stream/input)
- `ui/`: static browser UI (`xterm.js`)

## Quick Start

1. Start control plane:

```bash
cd cc-control
go run ./cmd/cc-control \
  -addr :18080 \
  -ui-dir ../ui \
  -agent-token agent-dev-token \
  -ui-token admin-dev-token
```

2. Start one agent on a server:

```bash
cd cc-agent
go run ./cmd/cc-agent \
  -control-url ws://127.0.0.1:18080/ws/agent \
  -agent-token agent-dev-token \
  -server-id srv-local \
  -allow-root /path/to/repo \
  -claude-path /absolute/path/to/claude-code
```

for example, in macOS

```bash
cd cc-agent
go run ./cmd/cc-agent \
  -control-url ws://127.0.0.1:18080/ws/agent \
  -agent-token agent-dev-token \
  -server-id srv-local \
  -allow-root /Users/xuzhougeng/Documents/agent-control/cc-agent \
  -claude-path /opt/homebrew/bin/claude
```

3. Open browser:

`http://127.0.0.1:18080`

Use UI token: `admin-dev-token`.

## What Works

- Server register + heartbeat online/offline
- Create session on selected server
- PTY stream to xterm.js and input roundtrip
- Resize + stop session (TERM then KILL)
- Prompt detection (`approve/reject`, `(y/n)`, etc.) with global pending queue
- Approve/Reject actions mapped to `y\n` / `n\n`
- JSONL audit log (`cc-control/audit.jsonl`)

## Security Baseline (MVP)

- Agent-side cwd whitelist (`-allow-root`)
- Command fixed to `claude-code` (control emits fixed cmd)
- Env allowlist/prefix on agent (`-env-allow-keys`, `-env-allow-prefix`)
- Separate agent/UI bearer tokens
- Basic per-token rate limiting on control plane

