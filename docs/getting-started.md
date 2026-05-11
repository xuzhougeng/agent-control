# Getting Started

Agent Control gives you four progressively richer ways to use it. Pick the one that matches what you're trying to do, then graduate as you need more.

```
1. cc-agent stand-alone     →  one binary + LLM key, REPL on your laptop
2. cc-agent + cc-control    →  same agent, now driven from a web / mobile UI
3. cc-proxy                 →  wrap your existing Claude Code / Codex / Gemini CLI
4. Hosted or self-hosted    →  use console.cc-remote.app, or run cc-control yourself
```

> Chinese readers: the same flow with full code samples is at [tutorial/](tutorial/) (5 min quickstart → production deploy → UI → providers).

## 1 · Run cc-agent locally (single binary)

`cc-agent` is the new server-ops agent (v0.7.0+). It runs the LLM loop itself and ships with 8 built-in tools (`bash`, `read`, `write`, `grep`, `glob`, `sysinfo`, `proclist`, `logtail`). Destructive commands hit an approval gate.

```bash
mkdir -p ~/cc-agent/yard ~/cc-agent/skills.d && cd ~/cc-agent
curl -LO https://github.com/xuzhougeng/agent-control/releases/latest/download/cc-agent-linux-amd64
chmod +x cc-agent-linux-amd64 && mv cc-agent-linux-amd64 cc-agent

# DeepSeek key (cheapest to start; swap providers later)
echo 'sk-xxxxxxxx' > ~/.cc-agent-key && chmod 600 ~/.cc-agent-key

CC_AGENT_API_KEY="$(cat ~/.cc-agent-key)" \
CC_AGENT_BASE_URL="https://api.deepseek.com" \
./cc-agent -provider deepseek -model deepseek-chat \
           -cwd ~/cc-agent/yard \
           -skills-dir ~/cc-agent/skills.d \
           -memory ~/cc-agent/sessions.db
```

You get a REPL prompt. Ask it to do something:

```
you> what's the kernel version on this box?
▶ bash {command=uname -r}
✓ exit_code=0  6.6.87.2-microsoft-standard-WSL2
Kernel version: 6.6.87.2-microsoft-standard-WSL2
```

That's the whole stand-alone story — **no cc-control, no UI, no remote**. Good for: one-off local automation, learning the tool surface, embedding cc-agent into a script.

Deep dive: [`cc-agent` module README](https://github.com/xuzhougeng/agent-control/blob/main/cc-agent/README.md), Chinese walkthrough at [tutorial/01-quickstart](tutorial/01-quickstart.md).

## 2 · Connect cc-agent to cc-control (web / mobile UI)

When you want to drive the agent from a browser or phone — or run it on a remote server — point it at a `cc-control` instance.

```bash
./cc-agent \
  -provider deepseek -model deepseek-chat \
  -control-url wss://cc-remote.app/ws/agent \
  -agent-token <your-agent-token> \
  -server-id srv-01 \
  -allow-root /path/to/repo \
  -memory /var/lib/cc-agent/sessions.db
```

Flags that change the picture:

- `-control-url` — registers this agent with a control plane.
- `-agent-token` — the credential issued by `cc-control` (see step 4 for how to get one).
- `-server-id` — how this node shows up in the UI (one cc-agent process = one node).
- `-allow-root` — restricts the bash tool's working directory.

Once connected, the agent appears in the UI with a purple `cc-agent` badge. Browser, native macOS/iOS, and Windows clients all share the same protocol.

Deep dive: [tutorial/02-deploy](tutorial/02-deploy.md) (production deployment), [tutorial/03-using-ui](tutorial/03-using-ui.md) (web / iOS / Windows UI).

## 3 · cc-proxy — wrap an external CLI agent

If you'd rather drive **Claude Code, Codex, or Gemini CLI** through Agent Control's UI (instead of cc-agent's own LLM loop), use `cc-proxy`. It's a PTY proxy that wraps the external CLI binary and surfaces it as a regular session in the same control plane.

```bash
./cc-proxy \
  -control-url wss://cc-remote.app/ws/agent \
  -agent-token <your-agent-token> \
  -server-id claude-node-01 \
  -claude-path /usr/local/bin/claude     # or codex, gemini CLI
```

cc-agent and cc-proxy can run side by side on the same control plane. Pick per node:

| You want… | Use |
|---|---|
| A self-driving server-ops agent (bash + read + LLM-decided actions) | `cc-agent` |
| To reuse the full `claude` / `codex` / `gemini` CLI you already have | `cc-proxy` |

> Naming note: before v0.7.0 `cc-proxy` was called `cc-agent`. The new self-developed agent took over the `cc-agent` name; the legacy proxy was renamed to `cc-proxy`. UI badge color tells them apart.

## 4 · Hosted control plane, or run your own

Two ways to get the cc-control side:

### Option A — Hosted: `console.cc-remote.app`

Sign up, create a tenant, and the console mints two tokens for you:

- **UI Token** — for the browser dashboard / iOS / macOS / Windows app.
- **Agent Token** — for `cc-agent` / `cc-proxy` to register from your machines.

Then run cc-agent / cc-proxy as in steps 2–3 with `-control-url wss://cc-remote.app/ws/agent` and the Agent Token. Login to the dashboard with the UI Token — your nodes show up immediately.

Open the console: <https://console.cc-remote.app>. iOS / macOS app: [App Store](https://apps.apple.com/us/app/cc-remote/id6759078097).

### Option B — Self-host cc-control

`cc-control` is open source (MIT). Run it on a Linux box, behind TLS or Cloudflare Tunnel, and point your agents at it.

- [Public-server deployment overview](deploy-public-server.md)
- [Direct HTTP](deploy-public-server/01-direct-http.md) · [TLS via Nginx + Let's Encrypt](deploy-public-server/02-tls.md) · [Cloudflare Tunnel](deploy-public-server/02a-cloudflare-tunnel.md)
- [Operations & monitoring](deploy-public-server/03-operations.md)
- [Run cc-agent as a background service](deploy-public-server/04-agent-background.md) (systemd / NSSM)

## What to read next

- [Use cases](use-cases.md) — when to pick what (server-ops, bioinformatics, multi-tenant, native apps).
- [Architecture](architecture.md) — components, protocol, lifecycle.
- [API reference](api.md) — REST/WS endpoints.
- [Tutorial 教程](tutorial/) — Chinese, step-by-step with full code blocks.
