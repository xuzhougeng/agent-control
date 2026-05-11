# cc-agent `!` Shell Escape Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `!cmd` (run shell, LLM does not see it) and `!!cmd` (run shell, buffer output to fold into the next user message) prefixes to the cc-agent REPL.

**Architecture:** Three small pieces in a new `cc-agent/internal/transport/shell.go`: `execShell` (timeout+capture wrapper over `/bin/sh -c`), `pendingShell` (FIFO buffer of `(cmd, out)` entries), and `foldShellContext` (pure formatter that prepends buffered shell context onto the next user input). All three are wired into `cli.go`'s REPL loop. The agent loop, memory schema, and tool registry are untouched.

**Tech Stack:** Go, `os/exec`, `context`, `chzyer/readline` (existing). Tests use stdlib `testing`.

**Spec:** `docs/superpowers/specs/2026-05-11-cc-agent-bang-shell-design.md`

---

## File Structure

- **Create** `cc-agent/internal/transport/shell.go` — `execShell`, `pendingShell`, `foldShellContext`. One file because the three pieces are small (< 30 lines each) and only ever used together.
- **Create** `cc-agent/internal/transport/shell_test.go` — unit tests for all three.
- **Modify** `cc-agent/internal/transport/cli.go`:
  - Add `!` / `!!` prefix dispatch in the REPL loop (around current line 213-225).
  - Drain buffer into normal user input before `agent.Run` (current line 233).
  - Extend `:help` text.

No other files change.

---

### Task 1: `execShell` helper + tests

`execShell` runs a single command through `/bin/sh -c`, streaming combined stdout+stderr live to the terminal, capturing them into a returned string, and honoring a context for cancellation. 60s default timeout. Captured output truncated to 16 KiB.

**Files:**
- Create: `cc-agent/internal/transport/shell.go`
- Create: `cc-agent/internal/transport/shell_test.go`

- [ ] **Step 1: Write the failing test (success case)**

Create `cc-agent/internal/transport/shell_test.go`:

```go
package transport

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestExecShell_Success(t *testing.T) {
	var live bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := execShell(ctx, "echo hello", "", &live, 2*time.Second, 1<<14)
	if err != nil {
		t.Fatalf("execShell: %v", err)
	}
	if !strings.Contains(got, "hello") {
		t.Errorf("captured output missing 'hello'; got=%q", got)
	}
	if !strings.Contains(live.String(), "hello") {
		t.Errorf("live stream missing 'hello'; got=%q", live.String())
	}
}
```

- [ ] **Step 2: Run the test, watch it fail**

Run: `cd cc-agent && go test ./internal/transport/ -run TestExecShell_Success -v`
Expected: FAIL — `execShell` undefined.

- [ ] **Step 3: Implement `execShell`**

Create `cc-agent/internal/transport/shell.go`:

```go
package transport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// execShell runs cmdStr through /bin/sh -c, streaming combined stdout+stderr
// to live (typically the readline Stdout) and capturing them into a returned
// string. The captured form is truncated to maxBytes with a marker, so the
// REPL buffer can safely fold huge command output into a memory write
// without bloating the next provider request. Live streaming is uncapped.
//
// timeout is enforced as a hard cap on top of ctx; when timeout <= 0 the
// caller's ctx alone gates the run.
func execShell(ctx context.Context, cmdStr, cwd string, live io.Writer, timeout time.Duration, maxBytes int) (string, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	if cwd != "" {
		cmd.Dir = cwd
	}
	var cap bytes.Buffer
	w := io.MultiWriter(live, &cap)
	cmd.Stdout = w
	cmd.Stderr = w
	runErr := cmd.Run()
	out := truncateBytes(cap.Bytes(), maxBytes)
	if ctx.Err() == context.DeadlineExceeded {
		return string(out) + "\n[timed out]\n", nil
	}
	if runErr != nil {
		return string(out) + fmt.Sprintf("\n[exit: %v]\n", runErr), nil
	}
	return string(out), nil
}

func truncateBytes(b []byte, maxBytes int) []byte {
	if maxBytes <= 0 || len(b) <= maxBytes {
		return b
	}
	cut := b[:maxBytes]
	return append(cut, []byte(fmt.Sprintf("\n…(+%d more bytes)\n", len(b)-maxBytes))...)
}
```

- [ ] **Step 4: Run the test, watch it pass**

Run: `cd cc-agent && go test ./internal/transport/ -run TestExecShell_Success -v`
Expected: PASS.

- [ ] **Step 5: Write the failing test (non-zero exit captures and reports)**

Add to `cc-agent/internal/transport/shell_test.go`:

```go
func TestExecShell_NonZeroExit(t *testing.T) {
	var live bytes.Buffer
	ctx := context.Background()
	got, err := execShell(ctx, "echo boom; exit 7", "", &live, 2*time.Second, 1<<14)
	if err != nil {
		t.Fatalf("execShell returned error; expected nil: %v", err)
	}
	if !strings.Contains(got, "boom") {
		t.Errorf("missing stdout in capture: %q", got)
	}
	if !strings.Contains(got, "[exit:") {
		t.Errorf("missing exit marker in capture: %q", got)
	}
}
```

- [ ] **Step 6: Run, watch pass**

Run: `cd cc-agent && go test ./internal/transport/ -run TestExecShell_NonZeroExit -v`
Expected: PASS.

- [ ] **Step 7: Write the failing test (timeout)**

Add to `cc-agent/internal/transport/shell_test.go`:

```go
func TestExecShell_Timeout(t *testing.T) {
	var live bytes.Buffer
	got, err := execShell(context.Background(), "sleep 5", "", &live, 200*time.Millisecond, 1<<14)
	if err != nil {
		t.Fatalf("expected nil err; got %v", err)
	}
	if !strings.Contains(got, "[timed out]") {
		t.Errorf("missing timeout marker: %q", got)
	}
}
```

- [ ] **Step 8: Run, watch pass**

Run: `cd cc-agent && go test ./internal/transport/ -run TestExecShell_Timeout -v`
Expected: PASS (within ~300ms).

- [ ] **Step 9: Write the failing test (truncation)**

Add to `cc-agent/internal/transport/shell_test.go`:

```go
func TestExecShell_Truncation(t *testing.T) {
	var live bytes.Buffer
	// Print 4KiB of 'x's; cap at 100 bytes.
	got, err := execShell(context.Background(), "printf 'x%.0s' $(seq 1 4096)", "", &live, 2*time.Second, 100)
	if err != nil {
		t.Fatalf("execShell: %v", err)
	}
	if !strings.Contains(got, "…(+") {
		t.Errorf("expected truncation marker; got=%q", got)
	}
	if len(live.String()) < 4096 {
		t.Errorf("live stream should be uncapped; got %d bytes", len(live.String()))
	}
}
```

- [ ] **Step 10: Run, watch pass**

Run: `cd cc-agent && go test ./internal/transport/ -run TestExecShell -v`
Expected: all four pass.

- [ ] **Step 11: Commit**

```bash
git add cc-agent/internal/transport/shell.go cc-agent/internal/transport/shell_test.go
git commit -m "feat(cc-agent): execShell helper for REPL ! prefix"
```

---

### Task 2: `pendingShell` buffer + `foldShellContext` formatter

`pendingShell` is a tiny FIFO of `(cmd, out)` entries. `foldShellContext` is the pure function that turns a drained buffer plus a raw user line into the single user message string the agent sees. Splitting fold-from-execution makes both trivially testable without standing up `RunCLI`.

**Files:**
- Modify: `cc-agent/internal/transport/shell.go`
- Modify: `cc-agent/internal/transport/shell_test.go`

- [ ] **Step 1: Write the failing test (buffer push/take)**

Append to `cc-agent/internal/transport/shell_test.go`:

```go
func TestPendingShell_PushTake(t *testing.T) {
	var p pendingShell
	if !p.empty() {
		t.Fatal("new buffer should be empty")
	}
	p.push("echo a", "a\n")
	p.push("echo b", "b\n")
	if p.empty() {
		t.Fatal("buffer should be non-empty")
	}
	got := p.take()
	if len(got) != 2 || got[0].cmd != "echo a" || got[1].cmd != "echo b" {
		t.Errorf("unexpected drain: %+v", got)
	}
	if !p.empty() {
		t.Fatal("take() should drain the buffer")
	}
}
```

- [ ] **Step 2: Run, watch it fail**

Run: `cd cc-agent && go test ./internal/transport/ -run TestPendingShell -v`
Expected: FAIL — `pendingShell` undefined.

- [ ] **Step 3: Implement `pendingShell`**

Append to `cc-agent/internal/transport/shell.go`:

```go
// shellEntry is one !! invocation captured for later folding into the next
// real user message. Strings already include any [exit:] / [timed out]
// markers added by execShell.
type shellEntry struct {
	cmd string
	out string
}

// pendingShell is a single-session FIFO of !! captures. Not goroutine-safe
// because RunCLI is single-threaded — the REPL goroutine is the only writer
// and reader.
type pendingShell struct {
	entries []shellEntry
}

func (p *pendingShell) push(cmd, out string) {
	p.entries = append(p.entries, shellEntry{cmd: cmd, out: out})
}

func (p *pendingShell) take() []shellEntry {
	out := p.entries
	p.entries = nil
	return out
}

func (p *pendingShell) empty() bool { return len(p.entries) == 0 }
```

- [ ] **Step 4: Run, watch pass**

Run: `cd cc-agent && go test ./internal/transport/ -run TestPendingShell -v`
Expected: PASS.

- [ ] **Step 5: Write the failing test (fold formatter)**

Append to `cc-agent/internal/transport/shell_test.go`:

```go
func TestFoldShellContext_NoEntriesReturnsRawInput(t *testing.T) {
	got := foldShellContext(nil, "hello")
	if got != "hello" {
		t.Errorf("empty buffer should pass input through; got=%q", got)
	}
}

func TestFoldShellContext_OneEntry(t *testing.T) {
	entries := []shellEntry{{cmd: "ls", out: "foo\nbar\n"}}
	got := foldShellContext(entries, "fix it")
	want := "[user-shell] $ ls\nfoo\nbar\n===\nfix it"
	if got != want {
		t.Errorf("fold mismatch:\nwant=%q\ngot =%q", want, got)
	}
}

func TestFoldShellContext_MultipleEntriesAndEmptyOutput(t *testing.T) {
	entries := []shellEntry{
		{cmd: "true", out: ""},
		{cmd: "echo y", out: "y\n"},
	}
	got := foldShellContext(entries, "now what")
	want := "[user-shell] $ true\n(no output)\n---\n[user-shell] $ echo y\ny\n===\nnow what"
	if got != want {
		t.Errorf("fold mismatch:\nwant=%q\ngot =%q", want, got)
	}
}
```

- [ ] **Step 6: Run, watch all three fail**

Run: `cd cc-agent && go test ./internal/transport/ -run TestFoldShellContext -v`
Expected: FAIL — `foldShellContext` undefined.

- [ ] **Step 7: Implement `foldShellContext`**

`shell.go` from Task 1 does not yet import `strings`. Add it. The import block becomes:

```go
import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)
```

Then append to `cc-agent/internal/transport/shell.go`:

```go
// foldShellContext renders a drained pendingShell buffer + the operator's
// fresh line into the single user message that gets handed to agent.Run.
// When entries is empty the raw line is returned unchanged so a session with
// zero !! usage is byte-identical to today.
func foldShellContext(entries []shellEntry, userLine string) string {
	if len(entries) == 0 {
		return userLine
	}
	var sb strings.Builder
	for i, e := range entries {
		if i > 0 {
			sb.WriteString("---\n")
		}
		sb.WriteString("[user-shell] $ ")
		sb.WriteString(e.cmd)
		sb.WriteByte('\n')
		out := e.out
		if out == "" {
			out = "(no output)\n"
		} else if !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		sb.WriteString(out)
	}
	sb.WriteString("===\n")
	sb.WriteString(userLine)
	return sb.String()
}
```

- [ ] **Step 8: Run, watch all three pass**

Run: `cd cc-agent && go test ./internal/transport/ -run TestFoldShellContext -v`
Expected: PASS.

- [ ] **Step 9: Run the full transport test suite once**

Run: `cd cc-agent && go test ./internal/transport/ -v`
Expected: PASS (no regressions in `esc_unix_test.go`).

- [ ] **Step 10: Commit**

```bash
git add cc-agent/internal/transport/shell.go cc-agent/internal/transport/shell_test.go
git commit -m "feat(cc-agent): pendingShell buffer and foldShellContext for !!"
```

---

### Task 3: Wire `!` and `!!` into the REPL loop

Plumb the three helpers into `cli.go`. Detect prefixes after `TrimSpace`, before the slash dispatch. ESC during a `!` run reuses `watchEscDuringRun`; SIGINT reuses `installRunInterrupt`. For normal-text turns, drain the buffer before `agent.Run`.

**Files:**
- Modify: `cc-agent/internal/transport/cli.go`

- [ ] **Step 1: Read the current REPL loop**

Re-read `cc-agent/internal/transport/cli.go` lines 200-245 (the `for { ... }` block). The change sits between the `:` dispatch and the `agent.Run` call.

- [ ] **Step 2: Declare the buffer at function scope**

Inside `RunCLI`, immediately after `defer rl.Close()` (~line 178), add:

```go
	var bangBuf pendingShell
```

- [ ] **Step 3: Add `!` / `!!` dispatch in the loop**

Find this block in `cli.go` (around line 217-225):

```go
		if line == "exit" || line == "quit" || line == ":exit" || line == ":quit" {
			return nil
		}
		if strings.HasPrefix(line, ":") {
			if err := handleSlashCommand(ctx, ag, rc, sessionID, line, approver); err != nil {
				fmt.Fprintf(rl.Stderr(), "command error: %v\n", err)
			}
			continue
		}
```

Insert immediately *after* it (before the `runCtx, cancelRun := ...` block):

```go
		// Shell-escape prefixes. !! captures output for the next turn; !
		// is a pure side-trip. Both skip the destructive-command approver
		// because the operator is hand-typing.
		if strings.HasPrefix(line, "!!") {
			cmd := strings.TrimSpace(strings.TrimPrefix(line, "!!"))
			if cmd == "" {
				fmt.Fprintln(rl.Stderr(), "usage: !! <command>   (run + buffer for next turn)")
				continue
			}
			runBang(ctx, rl, cmd, &bangBuf, true)
			continue
		}
		if strings.HasPrefix(line, "!") {
			cmd := strings.TrimSpace(strings.TrimPrefix(line, "!"))
			if cmd == "" {
				fmt.Fprintln(rl.Stderr(), "usage: ! <command>   (run shell; LLM does not see it)")
				continue
			}
			runBang(ctx, rl, cmd, nil, false)
			continue
		}
```

- [ ] **Step 4: Drain the buffer into the user line**

Replace this line (current line ~233):

```go
		_, runErr := ag.Run(runCtx, sessionID, line)
```

with:

```go
		_, runErr := ag.Run(runCtx, sessionID, foldShellContext(bangBuf.take(), line))
```

- [ ] **Step 5: Add the `runBang` helper at the bottom of `cli.go`**

Append to `cc-agent/internal/transport/cli.go` (after `parseNameVersion`):

```go
// runBang executes a single ! or !! command. When buf is non-nil, the
// (cmd, captured-output) pair is pushed into the buffer so the NEXT normal
// user turn can fold it in via foldShellContext. ESC during the run cancels
// the shell process via watchEscDuringRun; SIGINT routes the same way it
// does for agent.Run.
func runBang(parent context.Context, rl *readline.Instance, cmd string, buf *pendingShell, persist bool) {
	fmt.Fprintf(rl.Stdout(), "\033[90m$ %s\033[0m\n", cmd)
	runCtx, cancelRun := context.WithCancel(parent)
	defer cancelRun()
	stopSig := installRunInterrupt(cancelRun)
	defer stopSig()
	stopEsc, escErr := watchEscDuringRun(cancelRun)
	if escErr != nil {
		fmt.Fprintf(rl.Stderr(), "esc-watch: %v (continuing without ESC interrupt)\n", escErr)
		stopEsc = func() {}
	}
	defer stopEsc()

	out, err := execShell(runCtx, cmd, "", rl.Stdout(), 60*time.Second, 16<<10)
	if err != nil {
		fmt.Fprintf(rl.Stderr(), "shell error: %v\n", err)
		return
	}
	if persist && buf != nil {
		buf.push(cmd, out)
		fmt.Fprintln(rl.Stdout(), "\033[90m[buffered for next turn]\033[0m")
	}
}
```

- [ ] **Step 6: Ensure imports**

`cli.go` already imports `strings`, `context`, `fmt`, `time`, and `github.com/chzyer/readline`. No new imports needed. Verify with:

Run: `cd cc-agent && go build ./...`
Expected: build succeeds.

- [ ] **Step 7: Run the existing transport tests**

Run: `cd cc-agent && go test ./internal/transport/ -v`
Expected: PASS — including the ESC tests, the shell tests from Tasks 1-2.

- [ ] **Step 8: Run all cc-agent tests**

Run: `cd cc-agent && go test ./...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add cc-agent/internal/transport/cli.go
git commit -m "feat(cc-agent): wire ! and !! prefixes into the REPL loop"
```

---

### Task 4: Update `:help` text

The `:help` switch case lives in `handleSlashCommand`. Add two lines documenting `!` and `!!`. No code change beyond the help string.

**Files:**
- Modify: `cc-agent/internal/transport/cli.go`

- [ ] **Step 1: Locate the help block**

In `cli.go`, find the `case ":help", ":?":` block (~line 287-300). The current body prints a list of commands and ends with the line `ESC during a run                 cancel the current turn`.

- [ ] **Step 2: Add `!` and `!!` to the help text**

Replace this block:

```go
	case ":help", ":?":
		fmt.Println(`commands:
  :help                            show this help
  :tools                           list registered tools
  :skills                          list loaded skills
  :reflect <name> [description]    distill current session into a skill
  :registry [search]               list team skills
  :publish <name>                  push a local skill to the team registry
  :install <name>[@version]        fetch + install a team skill (with preview)
  :history <name>                  show all versions of a team skill
  :rollback <name> <version>       install a specific older version
  :rewind [N]                      drop the last N user turn(s) + replies (default 1)
  :quit | :exit | exit | quit | Ctrl+D   leave the REPL
  ESC during a run                 cancel the current turn`)
		return nil
```

with:

```go
	case ":help", ":?":
		fmt.Println(`commands:
  :help                            show this help
  :tools                           list registered tools
  :skills                          list loaded skills
  :reflect <name> [description]    distill current session into a skill
  :registry [search]               list team skills
  :publish <name>                  push a local skill to the team registry
  :install <name>[@version]        fetch + install a team skill (with preview)
  :history <name>                  show all versions of a team skill
  :rollback <name> <version>       install a specific older version
  :rewind [N]                      drop the last N user turn(s) + replies (default 1)
  :quit | :exit | exit | quit | Ctrl+D   leave the REPL
  ! <command>                      run a shell command (LLM does NOT see it)
  !! <command>                     run a shell command and fold (cmd, output)
                                   into the NEXT user message
                                   (each ! is a fresh subshell; cd / interactive
                                   commands like vim, less won't persist or work)
  ESC during a run                 cancel the current turn (or the current !)`)
		return nil
```

- [ ] **Step 3: Build and run all tests**

Run: `cd cc-agent && go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add cc-agent/internal/transport/cli.go
git commit -m "docs(cc-agent): document ! and !! in :help"
```

---

### Task 5: Manual smoke test

End-to-end verification in a real terminal. No new code; this confirms the feature actually works for the human.

**Files:** (none modified)

- [ ] **Step 1: Build the binary**

Run: `cd cc-agent && go build -o /tmp/cc-agent ./cmd/cc-agent`
Expected: build succeeds.

- [ ] **Step 2: Launch the REPL with deny-destructive (no API key needed for `!` tests)**

Run: `/tmp/cc-agent -deny-destructive`

Expected: prompt `you>` appears. (API calls will fail without a key — that is fine, we're only exercising `!`.)

- [ ] **Step 3: Test plain `!`**

Type at the prompt: `!ls`
Expected: `$ ls` echoed, the cwd listing follows, prompt returns. No `[buffered]` marker. No memory write.

- [ ] **Step 4: Test `!!` then a normal turn (requires API key)**

Re-launch with a valid `-config` or env so the LLM can be reached, e.g.:

`/tmp/cc-agent -deny-destructive -memory /tmp/bang-test.db`

Type: `!!echo MARKER_42`
Expected: `$ echo MARKER_42`, `MARKER_42`, `[buffered for next turn]`.

Then type: `what string did I just print in my shell?`
Expected: the assistant's reply references `MARKER_42`, proving the fold worked.

Verify the memory write directly:

```bash
sqlite3 /tmp/bang-test.db "SELECT content FROM messages WHERE role='user' ORDER BY id DESC LIMIT 1;"
```

Expected: the content starts with `[user-shell] $ echo MARKER_42` and ends with `what string did I just print in my shell?`.

If no API key is available, skip this step and rely on the unit tests of `foldShellContext` for fold-correctness coverage.

- [ ] **Step 5: Test ESC cancels a long `!`**

Type: `!sleep 30`
Press: `ESC`
Expected: within ~100ms, the sleep exits and the prompt returns (the shell prints nothing; `[exit:` marker may appear since SIGKILL produces a non-zero exit).

- [ ] **Step 6: Test empty `!` and `!!`**

Type: `!  ` (just bang and whitespace)
Expected: `usage: ! <command>` line, prompt returns.

Type: `!!`
Expected: `usage: !! <command>`.

- [ ] **Step 7: Test `:help` shows the new lines**

Type: `:help`
Expected: the printout includes the `! <command>` and `!! <command>` lines.

- [ ] **Step 8: Exit cleanly**

Type: `:quit`
Expected: `bye.`, process exits 0.

No commit for this task — it's a checklist, not a code change.

---

## Notes for the implementer

- **Do not change `agent.Run`, `memory.Store`, or any tool.** All new state is ephemeral REPL state inside `RunCLI`.
- **Do not add a `:bang-clear` slash command** or any other lifecycle command for the buffer — out of scope per spec. If you find yourself wanting one, the spec needs an update first.
- **`pendingShell` is intentionally not goroutine-safe.** `RunCLI`'s loop is single-threaded. Adding a mutex would be premature.
- **Live streaming is uncapped.** `truncateBytes` only protects the *captured* string that gets folded into a user message. Don't try to cap the live writer too.
- **`!` skips the approver intentionally** — see spec Mechanism section. Do not re-add an approver call.
- **`runBang` always uses `cwd=""`** so the helper relies on the process's cwd (which is the agent cwd at launch, since `cli.go` does not `os.Chdir`). If a follow-up wants cwd to track `cfg.Cwd`, plumb it through — but that is a separate change.
