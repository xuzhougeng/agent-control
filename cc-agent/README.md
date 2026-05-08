# cc-agent (server-ops agent)

A standalone Go agent for server operations. Owns its own LLM loop, tool
registry, and session persistence — not a wrapper around an external CLI.

> The previous module that wrapped Claude/Codex/Gemini PTYs has been renamed to
> `cc-proxy`. This module is the new agent.

## Quick start

```bash
export CC_AGENT_API_KEY=sk-ant-...
go run ./cmd/cc-agent \
  -provider anthropic \
  -model claude-sonnet-4-6 \
  -cwd /etc \
  -memory ~/.cc-agent/sessions.db
```

The CLI starts a REPL. Each line of input becomes one user turn; the agent
loop runs the model, executes any tool calls, streams events, and stops on
`end_turn` or after `MaxIterations` (default 12).

## Configuration

`-config path/to/config.json` overrides; flags override config; env overrides
both.

```json
{
  "provider": "anthropic",
  "model": "claude-sonnet-4-6",
  "cwd": "/var/log",
  "allowed_roots": ["/var/log", "/etc"],
  "max_iterations": 12,
  "max_tokens": 4096,
  "memory_path": "/var/lib/cc-agent/sessions.db",
  "skills_dir": "/etc/cc-agent/skills.d"
}
```

Env vars (highest precedence):

| Variable                | Purpose                                        |
|-------------------------|------------------------------------------------|
| `CC_AGENT_PROVIDER`     | provider name                                  |
| `CC_AGENT_MODEL`        | model name                                     |
| `CC_AGENT_API_KEY`      | provider API key (also `ANTHROPIC_API_KEY`)    |
| `CC_AGENT_BASE_URL`     | OpenAI-compat base URL                         |
| `CC_AGENT_MEMORY_PATH`  | SQLite session DB path                         |
| `CC_AGENT_SKILLS_DIR`   | directory of skill JSON files                  |

## Permission modes

For destructive shell commands (`rm -rf`, `rm <files>`, `mkfs`, `dd of=/dev/sd*`,
`shutdown`, `systemctl stop|disable|mask`, fork bombs, `git push --force`, …)
the bash tool consults an `Approver` before exec.

| Mode                          | Approver                          | Behavior                                                              |
|-------------------------------|-----------------------------------|-----------------------------------------------------------------------|
| CLI REPL (default)            | `transport.CLIApprover`           | Print command + reason on stderr, prompt `[y/N]`                      |
| `-control-url` (default)      | `transport/control.RemoteApprover`| Send `approval_request` to cc-control → operator clicks Approve/Reject in UI Pending Approval card |
| `-full-permission`            | `tools.AlwaysApprove`             | Skip all prompts (yolo mode for trusted scripted runs)                |
| `-deny-destructive`           | `tools.AlwaysDeny`                | Auto-deny without prompt (safe for non-interactive HTTP/cron)         |

`-full-permission` and `-deny-destructive` win over the cc-control bridge if
both are set.

When denied (timeout, operator rejected, or `-deny-destructive`), the model
sees a `DENIED by operator (reason: …)` string as the tool result and is
free to retry with a safer alternative.

The destructive-command list is in `internal/tools/approval.go`; extend it
by appending to `dangerousMatchers` with a regex + reason.

## Tools

Bundled tools (`tools.DefaultRegistry`):

| Tool      | Purpose                                              |
|-----------|------------------------------------------------------|
| `bash`    | exec shell command (cwd-fixed, timeout-bound)        |
| `read`    | read text file (path containment via allowed_roots)  |
| `write`   | write file (overwrite=false by default)              |
| `grep`    | RE2 regex search across a directory tree             |
| `glob`    | list files matching a glob                           |
| `sysinfo` | uname / uptime / mem / disk snapshot                 |
| `proclist`| top processes by RSS                                 |
| `logtail` | last N lines of a file                               |

Add your own by implementing `tools.Tool` and registering on the registry.

## Provider support

| Provider          | Notes                                              |
|-------------------|----------------------------------------------------|
| `anthropic`       | Messages API + native tool_use                     |
| `openai`          | `/v1/chat/completions` with function calling       |
| `deepseek`        | OpenAI-compat (`CC_AGENT_BASE_URL=https://api.deepseek.com`) |
| `qwen`            | OpenAI-compat                                      |
| any vLLM/local    | OpenAI-compat                                      |

Reasoning models (DeepSeek `deepseek-v4-pro`, Qwen thinking variants) emit a
`reasoning_content` block alongside their text; cc-agent captures it and
echoes it back on the next turn so the model's chain-of-thought stays
consistent across tool calls.

## Persistence

- `-memory ""` (default): in-memory only; lost on exit.
- `-memory path.db`: SQLite-backed; sessions and full message history persist.

## Skills

A skill bundles a system-prompt fragment, a tool whitelist, and example
requests so the agent can be primed for a recurring task. Skills are JSON
files under `skills_dir/`:

```json
{
  "name": "nginx-triage",
  "description": "Triage common nginx issues",
  "prompt": "Focus on access.log/error.log under /var/log/nginx; suspect 5xx first.",
  "tools": ["bash", "read", "grep", "logtail"],
  "examples": ["nginx is returning 502 — diagnose"]
}
```

### Self-evolution (`:reflect`)

After a successful task, distill the session into a reusable skill from the
REPL:

```
you> use bash to run uname -r and report only the version
[tool runs, model answers]
you> :reflect kernel-probe Identify the running kernel
distilling session into skill...
✓ saved skill kernel-probe -> /etc/cc-agent/skills.d/kernel-probe.json
  description: Identify the running kernel
  tools: bash
  examples:
    - What kernel version am I running?
    - Show me only the kernel release string.
```

The reflector hands the LLM the session transcript (tool results truncated)
plus an instruction to emit a strict JSON skill object. Tool whitelist is
taken from the actual tools used during the session, not from the model's
output. Saved skills are immediately available via `:skills`, and any
future `cc-agent` start with the same `-skills-dir` reloads them.

REPL slash commands:

| Command                            | Action                                  |
|------------------------------------|------------------------------------------|
| `:help`                             | show all commands                        |
| `:skills`                           | list loaded skills                       |
| `:reflect <name> [description]`     | distill the current session into a skill |

## HTTP API (optional)

```bash
go run ./cmd/cc-agent -http :19090 -memory ./sessions.db &
curl -N -XPOST localhost:19090/run \
  -H 'Content-Type: application/json' \
  -d '{"session_id":"alpha","input":"check disk usage"}'
```

Responses stream as NDJSON events (`kind=user|assistant|tool_call|tool_result|error`).

## cc-control bridge

When `-control-url` is set, cc-agent registers with the control plane over
WebSocket and accepts chat sessions from the web/iOS/Windows UIs. Only chat
mode is supported (PTY/terminal stays in `cc-proxy`); a `start_session`
inbound is rejected explicitly.

```bash
./cc-agent \
  -provider deepseek -model deepseek-chat \
  -cwd /var/log -allowed-roots /var/log \
  -control-url ws://your-control:18180/ws/agent \
  -agent-token <agent-token> \
  -server-id ops-01 \
  -deny-destructive
```

When `-control-url` (or `-http`) is set, the binary runs as a daemon — the
local REPL is skipped and the process stays up until SIGINT/SIGTERM.

Each agent turn streams to the UI as multiple `chat_msg` events:

- one **progress** bubble per `tool_use` (e.g. `▶ bash {command=...}`)
- one **progress** bubble per tool result (e.g. `✓ exit_code: 0 ...`)
- one **final** bubble with the assistant's full text and a structured
  `meta.operations` summary

The UI's existing `.progress` styling distinguishes intermediate from final
without any frontend change.

## Status

- [x] Provider abstraction (Anthropic + OpenAI-compat)
- [x] ReAct main loop with tool dispatch
- [x] Default ops tool set
- [x] Session persistence (in-memory + SQLite)
- [x] Skills loader + self-evolution (`:reflect`)
- [x] CLI REPL + HTTP API
- [x] Approval gate for destructive bash commands — CLI prompt, UI-routed via cc-control, or `-full-permission` / `-deny-destructive`
- [x] `reasoning_content` round-trip for thinking-mode providers
- [x] cc-control WS chat bridge (chat-mode only)
- [x] Step-level streaming to UI (one progress bubble per tool step)

## Roadmap

- [ ] **True token streaming (SSE)**: provider-side `stream:true` with
  delta accumulation. Requires UI dedup by `message_id` so partial deltas
  update one bubble in place instead of appending new ones.
- [ ] **Approval webhook (Slack / email)**: extend the existing UI-routed
  approval to also notify operators via webhook when they're not in the UI.
  The protocol is in place; just need an outgoing pusher.
- [ ] **Multi-agent coordination**: a coordinator agent decomposes a
  request, dispatches subtasks to specialist sub-agents (each scoped to a
  skill), then merges their results — EvoMaster-style.
- [ ] **Auto-reflect heuristic**: opt-in flag that triggers `:reflect` on
  every Nth successful task, gated by token-cost / similarity to existing
  skills.
- [ ] **Approval audit trail**: persist approval decisions (operator,
  command, reason, outcome) to a JSONL file for after-the-fact review.
- [ ] **Per-skill model override**: a skill may pin to a cheaper/faster
  provider when its task is well-defined (e.g. log triage on local Qwen).
- [ ] **Tool sandboxing**: run `bash` inside a namespace/jail when the
  agent is operating without a human in the loop.
- [ ] **Memory compaction**: when a session's history exceeds a threshold,
  summarize older turns into a single system note to keep context cost
  bounded.
- [ ] **Resume across restarts**: `-session <id>` already replays history
  from SQLite; add automatic resume of any in-flight chat session bound to
  a cc-control instance after agent restart.

## Acknowledgments

This module is **not original work**. It is a Go port that adapts ideas
from existing open-source agent frameworks for a server-operations use
case. We owe most of the architecture to:

- [**lsdefine/GenericAgent**](https://github.com/lsdefine/GenericAgent) —
  the *agent loop + skill tree + memory + reflect/self-evolution* model
  comes directly from this project. Their Python `agent_loop.py`,
  `llmcore.py`, `plugins/`, `memory/`, and `reflect/` layout is what
  `internal/agent/`, `internal/llm/`, `internal/skills/` (including
  `:reflect`), and `internal/memory/` are translated from. The "small seed
  + skill tree that grows" philosophy is theirs.
- [**sjtu-sai-agents/EvoMaster**](https://github.com/sjtu-sai-agents/EvoMaster) —
  the *foundational evolving agent framework* shape (configs / extensions /
  playground separation, plus the long-term goal of self-improving
  specialist agents) informed how we split packages and what landed in
  the Roadmap (auto-reflect, multi-agent coordination, per-skill model
  override).

Other influences:

- The **ReAct** loop pattern (reason → act → observe → repeat) follows
  Yao et al., *ReAct: Synergizing Reasoning and Acting in Language Models*
  (2022).
- **Anthropic Claude** — the `tool_use` / `tool_result` content-block
  protocol our `internal/llm/anthropic.go` speaks is from Anthropic's
  Messages API. Our default ops tool surface (`bash`, `read`, `write`,
  `grep`, `glob`) intentionally mirrors Claude Code's tool naming so an
  operator switching between the two has nothing to relearn.
- **OpenAI** — the function-calling shape in `internal/llm/openai.go` is
  the OpenAI Chat Completions tool format, also used by DeepSeek, Qwen,
  vLLM, llama.cpp, and Ollama.
- **DeepSeek** — the `reasoning_content` round-trip handling exists
  specifically to support DeepSeek's thinking-mode models.

What this project actually contributes is a **focused Go rewrite** with:

- a single static binary (no Python runtime) suitable for dropping onto a
  server,
- chat-mode-only integration with the existing `cc-control` plane so the
  operator can drive multiple agent hosts from one web/iOS/Windows UI,
- an opinionated *server operations* tool set + destructive-command
  approval gate, rather than the more general computer-control surface of
  the upstream projects.

If you build on top of cc-agent, please continue to credit the upstream
projects above — most of the hard design thinking happened there first.
