# cc-agent `!` Shell Escape — Design

**Status:** Draft
**Date:** 2026-05-11
**Component:** `cc-agent/internal/transport` (REPL)

## Goal

Let the operator run shell commands directly from the cc-agent REPL — without round-tripping through the LLM — and decide per command whether the LLM should see what was run.

The motivating frustration: today, if the operator wants to check `git status` or `ls` mid-session, they either type a sentence and pay an LLM turn, or drop out of the REPL. Both are slow. A `!` prefix is the conventional answer (bash `!`, vim `:!`, psql `\!`).

The non-obvious question is **how the side-trip interacts with the LLM's view of the session**. This spec settles that explicitly.

## Mechanism

Two prefixes are intercepted in `RunCLI` before reaching `agent.Run`:

| Input          | LLM sees it? | Approver? | Behavior                                                                        |
| -------------- | ------------ | --------- | ------------------------------------------------------------------------------- |
| `!cmd`         | no           | no        | run `cmd` in agent cwd, stream output to terminal, return to prompt             |
| `!!cmd`        | next turn    | no        | same as `!cmd`, plus buffer `(cmd, output)` to fold into the next user message  |
| (other input)  | yes          | (n/a)     | normal `agent.Run`. If buffer non-empty, prepend folded shell context, clear it |
| `:slash`       | (slash)      | (n/a)     | unchanged                                                                       |

`!!` is chosen knowing it shadows bash's history-expansion idiom; readline in cc-agent does not perform bash-style `!!` expansion, so there is no functional conflict — only a conceptual one to document.

The destructive-command approver (`tools.DangerousReason` + `CLIApprover`) is **not** invoked for `!` / `!!`. Rationale: the operator is hand-typing the command in their own shell-style escape. Approvals exist to catch the *LLM* doing something destructive; they would only be friction here. The LLM's own `bash` tool calls continue to go through the approver unchanged.

## Where it lives

All new logic is scoped to `cc-agent/internal/transport`:

- `cli.go:202` — main REPL loop: add prefix detection branches before slash / agent.Run.
- New file `cli_shell.go`:
  - `execShell(ctx, cmd, cwd string, stdout, stderr io.Writer) (combinedOutput string, err error)` — runs `/bin/sh -c cmd`, streams stdout+stderr to the supplied writers AND captures them into a returned buffer. 60s default timeout. Captured output truncated to 16 KiB (with `…(+N more bytes)` marker) before being returned to the buffer — keeps memory writes bounded.
  - `pendingShell` type and helpers (see below).

The `agent.Agent`, `memory.Store`, and `tools.Bash` packages are **not** modified. The buffer is ephemeral REPL state.

## Buffer

```go
type pendingShell struct {
    entries []shellEntry
}
type shellEntry struct {
    cmd string
    out string
}

func (p *pendingShell) push(cmd, out string) { ... }
func (p *pendingShell) take() []shellEntry { ... } // returns + clears
func (p *pendingShell) empty() bool { ... }
```

One instance per `RunCLI` invocation, captured by the loop. Not shared across sessions because `RunCLI` is single-session.

Lifetime: across consecutive `!!` calls, until the next real user input drains it, or the REPL exits (silently discarded — they were never committed to memory).

## Fold format

When the buffer is non-empty and the operator submits a normal line, the user message sent to `agent.Run` becomes:

```
[user-shell] $ <cmd1>
<out1>
---
[user-shell] $ <cmd2>
<out2>
===
<original user line>
```

- One block per `!!` entry, separated by `---`.
- `===` separates the shell context from the operator's actual prompt.
- If `<out>` is empty, the line `<out>` is replaced by `(no output)`.

This single string is what `agent.Run` receives. It is what gets persisted to `memory.Store` as one user message, what the router sees, and what the LLM sees on the next turn. Memory invariants stay unchanged: still alternating user → assistant → tool (no two consecutive user records).

## Display

- `!cmd` and `!!cmd` both echo a `$ <cmd>` line in dim color before streaming output, so the transcript on the operator's screen looks like a normal shell.
- `!!cmd` prints `[buffered for next turn]` in a dim color after the command completes, so the operator knows the context is staged.
- If the operator types a third `!!cmd` while two are already buffered, all three accumulate; the next normal turn drains them all.

## Help and completion

- `:help` gains:
  ```
    ! <cmd>                          run a shell command (LLM does NOT see it)
    !! <cmd>                         run a shell command and inject (cmd, output) into the next user turn
  ```
- `makeCompleter` is **not** modified. `!` is not a slash command and shell completion is out of scope.

## Edge cases and limitations

- **`!cd foo`** does not persist across `!` invocations; each call is a fresh `/bin/sh -c` subprocess. Documented in `:help`.
- **Interactive commands** (`!vim`, `!less`, `!ssh`) will not work — stdin is not wired through. The command will hang or exit immediately depending on TTY detection. v1: documented, not blocked. The 60s timeout backstops a wedged process.
- **Timeout** — fixed 60s in v1. No flag. If operator needs longer, run in the host shell.
- **Output size** — truncated at 16 KiB *for the captured buffer*. Live terminal streaming is uncapped. Truncation only protects the eventual memory write.
- **Empty `!`** — `!` followed by whitespace prints a help hint and re-prompts (does not run an empty shell).
- **`!!` with no command** — same as empty `!`.
- **REPL-only** — the daemon paths (`-control-url`, `-http`) do not run `RunCLI`, so `!` is inherently scoped to the local REPL. No code in those paths needs to change.
- **`:rewind` interaction** — buffered-but-not-yet-injected entries are unaffected by `:rewind`. Once an entry is folded into a user message and committed to memory, it's part of that message and rewinds with it. No special handling needed.
- **ESC mid-`!`** — ESC during a `!` run cancels the shell command via the same `watchEscDuringRun` mechanism already used for `agent.Run`. Reuse that helper.
- **SIGINT** — the same `installRunInterrupt` SIGINT routing applies during `!` execution. Documented but no behavior change beyond pointing the cancel at the shell process.

## Out of scope (v1)

- Persistent shell session (would require running `/bin/sh -i` and a pty).
- TTY pass-through for interactive commands.
- A `!!!` (or similar) to *replace* a buffered entry vs. append.
- Editing or clearing the buffer mid-session via a slash command. (If real demand emerges, add `:bang-clear` later.)
- History-search of `!` lines as a separate stream (they live in `~/.cc-agent/history` mingled with everything else, and `Ctrl+R` finds them fine).

## Testing

- Unit test for `execShell`: success, non-zero exit, timeout, large output truncation.
- Unit test for `pendingShell`: push, take, empty.
- Unit test for `foldShellContext`: empty buffer passthrough, single entry, multiple entries with empty-output fallback.
- Manual smoke against the built binary: confirms wiring (`!`, `!!`, ESC, empty-arg, `:help`). A full `RunCLI`-level integration test is out of scope for v1 — it would require a PTY harness and readline mocking, which is significant scaffolding for code paths that are mechanical (HasPrefix + helper call + buffer drain).

## Migration / compat

- No config changes.
- No memory schema changes.
- No skill / tool registry changes.
- A user who never types `!` is unaffected.
