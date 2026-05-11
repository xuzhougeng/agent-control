# cc-agent Manual Injection — Design

**Status:** Draft
**Date:** 2026-05-11
**Component:** `cc-agent/internal/transport` (REPL), `cc-agent/internal/agent` (loop)

## Goal

Two REPL escapes that let the operator hand-inject material the router/LLM otherwise wouldn't see:

1. `@<path>` — attach a file's contents to the next user turn as context.
2. `:use <skill>` — force a specific skill onto the next turn, bypassing the auto-router.

The motivating frustration: today, if the operator wants to feed a log file into the conversation, they paste it manually (or pipe through `!!cat`, which works but reads weird). And there's no way to say "use *this* skill for the next question" — the LLMRouter decides, full stop. Both are common operator needs that currently require workarounds.

This spec is paired with the `!` shell-escape spec ([2026-05-11-cc-agent-bang-shell-design.md](2026-05-11-cc-agent-bang-shell-design.md)). The two share a design principle: **each REPL prefix binds to exactly one domain.**

## Symbol allocation

| Prefix              | Domain                       | LLM-visibility               |
| ------------------- | ---------------------------- | ---------------------------- |
| `:cmd`              | REPL meta / session state    | not visible (purely local)   |
| `!cmd` / `!!cmd`    | local subprocess             | invisible / next-turn        |
| `@<path>`              | file system               | next-turn (file content)     |

Why split skill-selection (under `:`) from file-attach (under `@`):

- `:use <skill>` is a **name-only switch** (no payload) — fits next to `:reflect <name>`, `:rewind`. Same shape, same family.
- `@<path>` is **payload-only** — the path *is* the content. Merging both into `@<name>` would require runtime "is this a registered skill or a path?" disambiguation, which breaks when a skill and a file share a name.
- `@` matches existing ecosystem convention (Cursor, Continue, Aider) for "attach file."

## Part 1: `@<path>` — file attach

### Mechanism

Single prefix, with an optional trailing prompt for the common "attach + ask" case. There is no `@@` doubled form — unlike `!` vs `!!` (which toggles LLM visibility), an attached file is always intended for the LLM, so the doubled form would only differ in timing, which the trailing-prompt shape covers more naturally.

| Input               | LLM sees?  | Approver? | Behavior                                                                                          |
| ------------------- | ---------- | --------- | ------------------------------------------------------------------------------------------------- |
| `@<path>`           | next turn  | no        | read file (path resolved against agent cwd), buffer `(path, content)`, return to prompt          |
| `@<path> <text>`    | this turn  | no        | read file + use `<text>` as the prompt; fold and dispatch immediately as one user turn            |
| (other input)       | yes        | (n/a)     | normal `agent.Run`. If buffer non-empty, prepend folded file context, clear it                    |

The destructive-command approver is **not** invoked — the operator is hand-typing the path, and file reads are read-only. The safety case is even weaker than for `!`.

The buffer shares the same lifecycle slot as the `!!` shell buffer (see "Buffer" below). One drain consumes both kinds, in order entered.

### Where it lives

All new logic in `cc-agent/internal/transport`:

- `cli.go:202` (REPL loop): add a single `@` prefix detection branch alongside the existing `!` / `!!` branches. Whether to fold-and-dispatch (trailing text present) or buffer-and-return (trailing text absent) is decided inside this branch.
- New file `cli_attach.go`:
  - `readAttach(path, cwd string) (content string, truncated bool, err error)` — resolves `~` and joins relative paths against `cwd` (not the operator's `pwd`, so behavior matches what tools see). Reads up to 256 KiB; on overflow, truncates and sets `truncated = true`.
  - Path resolution rules and the binary-file check live here so they're unit-testable without a REPL.

The `pendingShell` buffer type defined in `cli_shell.go` (per the bang-shell spec) is **renamed** to `pendingInject` and generalized to carry a sum of `shellEntry | attachEntry`. The bang-shell implementation has not yet landed (the plan exists but is unmerged), so the rename costs one search-and-replace there.

`agent.Agent`, `memory.Store`, and `skills.Registry` are not modified.

### Buffer

```go
type pendingInject struct {
    entries []injectEntry
}
type injectEntry struct {
    kind string // "shell" | "attach"
    // shell fields
    cmd string
    out string
    // attach fields
    path    string
    content string
    truncated bool
}

func (p *pendingInject) pushShell(cmd, out string)
func (p *pendingInject) pushAttach(path, content string, truncated bool)
func (p *pendingInject) take() []injectEntry // returns + clears
func (p *pendingInject) empty() bool
```

One instance per `RunCLI` invocation, captured by the loop.

Lifetime: across consecutive `!!` / `@` entries, until the next real user input drains it, or the REPL exits (silently discarded).

### Fold format

When the buffer is non-empty and the operator submits a normal line:

```
[user-shell] $ <cmd>
<out>
---
[user-attach] @<path>
<content>
===
<original user line>
```

- One block per entry, in entry order, separated by `---`.
- `[user-attach]` header makes file attaches distinguishable from shell outputs in the LLM's view, the router's view, and saved memory.
- `===` separates injected context from the operator's prompt.
- Single user message, persisted intact. Memory invariants (alternating user/assistant/tool) unchanged.
- Truncated attaches include a trailing `…(+N more bytes, truncated)` marker inside the `<content>` block.

### Display

- `@<path>` alone echoes `@<path> (N bytes)` in a dim color, then `[buffered for next turn]`.
- `@<path> <text>` echoes `@<path> (N bytes)` then proceeds straight to the turn with `<text>` as the prompt.
- A missing-file or oversized-error path prints an error and does **not** touch the buffer.

### Edge cases

- **`@<dir>`** — rejected with a hint to use a glob (out of scope for v1).
- **Binary files** — detected via UTF-8 validity check on the first 4 KiB. Rejected with a message; if the operator really wants bytes, `!!base64 <file>` works.
- **Symlinks** — followed. No special handling.
- **Size cap** — 256 KiB per file. Truncated with marker; live console echo notes `(N bytes, truncated to 256 KiB)`.
- **Multiple `@` entries before a real turn** — accumulate, drain together.
- **Empty `@`** — print a help hint and re-prompt.
- **Permission errors** — OS error shown, buffer untouched.
- **`:rewind` interaction** — same as `!!`: buffered-but-undrained entries are unaffected; once drained they're part of the user message and rewind with it.
- **ESC / SIGINT mid-read** — file reads are fast; not interrupting is acceptable. Documented, not blocked.

## Part 2: `:use <skill>` — skill override

### Mechanism

One-shot router override. Applies to the **next** normal user input.

| Input                          | Behavior                                                                                   |
| ------------------------------ | ------------------------------------------------------------------------------------------ |
| `:use <skill>`                 | verify `<skill>` is in the registry; stash `nextSkill = <name>`; print confirmation        |
| `:use <skill> <prompt-text>`   | combined form: stash `<skill>` and immediately dispatch `<prompt-text>` pinned to it       |
| (next bare input, pinned)      | skip `Router.Route`; wrap with the stashed skill via `skills.WrapUserInput`; clear pin     |
| `:use`                         | print `nextSkill` if set, else `(no skill pinned)`                                         |
| `:use -`                       | clear `nextSkill`                                                                          |

Why one-shot, not sticky:

- Sticky overrides are easy to forget and lead to confusing later turns.
- If the operator needs the same skill repeatedly, that's a signal the router needs tuning, not that the override should persist.
- One-shot mirrors `:reflect` and `:rewind` — same family rhythm.

### Where it lives

- `cli.go:283` (slash-command dispatch): new `case ":use":` handler.
- `cli.go:202` (REPL loop): before calling `agent.Run`, if `nextSkill` is set and the input is a bare line, resolve the skill and route to a new `agent.Agent` entry point that **bypasses the router**.
- `internal/agent/loop.go`: extract the existing routing block (lines 213–233) into a small helper, and add:
  ```go
  // RunWithForcedSkill runs one turn pinned to the given skill, bypassing the
  // auto-router. nil skill is equivalent to RunWithListener (no wrap, no route).
  func (a *Agent) RunWithForcedSkill(ctx context.Context, sessionID, userInput string, skill *skills.Skill, listener EventListener) (string, error)
  ```
  Internally: identical to `RunWithListener` except the router block is replaced with a direct `skills.WrapUserInput(userInput, skill)`. Caches still hit; persisted format is byte-identical to a router-picked turn (same `<skill name=…>…</skill>` wrap), so memory replay is unaffected.

The router skip is critical — without it, a `:use foo`-pinned turn would still be sent through the router, which might pick `bar` and double-wrap. The new entry point is the only way to guarantee a single, deterministic wrap.

### Display

- `:use <skill>` → `\033[32m✓ next turn pinned to skill\033[0m <name>` (one line).
- Normal turn under a pin: emit `EventRouter` with text `<name> (pinned)` so transcripts make clear the skill was operator-chosen, not auto-picked.
- `:use <unknown>` → red error, pin unchanged.

### Edge cases

- **Unknown skill name** — error, `nextSkill` not modified.
- **`:use <skill>` followed by `:rewind`** — `:rewind` only touches memory; the pin is REPL state and survives. Documented; `:use -` clears it.
- **`:use <skill>` followed by `@<path>`** — file buffer drains normally on the next bare line; the pinned skill applies to that drained turn. Both stack cleanly.
- **`:use <skill>` then `:use <other>`** — second one wins, no warning needed (it's REPL-only state).
- **`:use <skill>` then exit/quit** — pin discarded (ephemeral).
- **Interaction with `--no-route`** — `:use` still works; it doesn't go through the (disabled) router anyway. Effectively, `:use` is the only way to apply a skill when `--no-route` is set.

## Help and completion

`:help` gains:

```
  @<path>                          attach a file to the next user turn
  @<path> <text>                   attach a file and send <text> as the prompt now
  :use <skill> [prompt]            force a specific skill on the next turn
  :use                             show the pinned skill
  :use -                           clear the pinned skill
```

`makeCompleter` adds `:use` completion with dynamic skill names (reuse the `localNames` helper already used by `:reflect`). `@` is not added — it's a prefix, not a slash command. File-path completion under `@<TAB>` is out of scope for v1.

## Out of scope (v1)

- `@<glob>` (multi-file attach). Operator can call `@` multiple times.
- `@<url>` (URL fetch). Different domain; warrants its own discussion.
- Inline `@<path>` substitution within a prompt (Cursor-style: `look at @foo.go what does it do?`). Considered; rejected for v1 because `@` appears legitimately in real text (emails, handles), so safe tokenization needs more thought. Whole-line prefix sidesteps the issue.
- Sticky `:use` pin (until cleared). One-shot first; revisit if demand emerges.
- `@` for team-registered skills not yet installed. Operator must `:install` first.
- `@<TAB>` path completion.

## Testing

- Unit test for `readAttach`: success, missing file, binary file (UTF-8 invalid), oversize (truncation), symlink, `~` expansion, directory-rejected.
- Unit test for the buffer sum-type fold: shell-only, attach-only, mixed-order, truncated attach.
- Unit test for `:use` dispatch: known skill, unknown skill, no args (show), `-` (clear), combined form with inline prompt.
- Unit test for `Agent.RunWithForcedSkill`: verifies router is skipped, the persisted user message is byte-identical to what a router pick would have produced.
- Manual smoke: `@file`, `@file explain this`, `@huge` (truncation), `:use foo`, `:use foo prompt`, `:use bad`, mixed `@foo.go` then `:use kernel-probe explain`.

A full `RunCLI`-level integration test is out of scope for v1 (same reason as bang-shell — PTY harness scaffolding).

## Migration / compat

- No memory schema changes.
- No skill JSON / registry changes.
- The `pendingShell` → `pendingInject` rename is coordinated with the bang-shell PR (currently unmerged); if landed first, this spec absorbs the rename as a search-and-replace.
- `Agent.RunWithForcedSkill` is a pure addition; the existing `Run` / `RunWithListener` entry points are untouched.
- Operators who never type `@` or `:use` are unaffected.
