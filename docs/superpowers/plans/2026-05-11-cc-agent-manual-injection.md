# cc-agent Manual Injection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `@<path>` file attach (with optional inline prompt) and `:use <skill>` one-shot router override to the cc-agent REPL.

**Architecture:** Two independent features sharing one design principle (one prefix per domain). `@` extends the existing `pendingShell` buffer into a unified `pendingInject` carrying ordered shell+attach entries; the fold formatter is generalized accordingly. `:use` adds a one-shot REPL state (`nextSkill`) and a new agent entry point `RunWithForcedSkill` that bypasses the auto-router. Memory schema, tool registry, and skill schema are untouched.

**Tech Stack:** Go, stdlib `os`, `unicode/utf8`, `context`. Tests use stdlib `testing`. Builds on the already-landed `!` / `!!` machinery (`internal/transport/shell.go`).

**Spec:** `docs/superpowers/specs/2026-05-11-cc-agent-manual-injection-design.md`

---

## File Structure

- **Modify** `cc-agent/internal/transport/shell.go` — rename `pendingShell` → `pendingInject`, replace `shellEntry` with a unified `injectEntry`, rename `foldShellContext` → `foldInject`, add `pushShell` / `pushAttach` methods.
- **Modify** `cc-agent/internal/transport/shell_test.go` — same renames in tests; add cases for mixed-kind entries and attach-only.
- **Create** `cc-agent/internal/transport/attach.go` — `readAttach` (path resolve + read + UTF-8 check + truncation).
- **Create** `cc-agent/internal/transport/attach_test.go` — table-driven tests for `readAttach`.
- **Modify** `cc-agent/internal/agent/loop.go` — extract the routing block into a helper, add `RunWithForcedSkill`.
- **Modify** `cc-agent/internal/agent/loop_test.go` — test that `RunWithForcedSkill` skips the router and emits the right event.
- **Modify** `cc-agent/internal/transport/cli.go`:
  - Rename references to `pendingShell` / `bangBuf` (mechanical; type rename only).
  - Add `@` prefix branch in the REPL loop (stage form + inline-prompt form).
  - Add `nextSkill` state and converge user-turn dispatch through one helper.
  - Add `case ":use":` in `handleSlashCommand`.
  - Add `:use` to `makeCompleter` with dynamic skill-name completion.
  - Update `:help` text.

No other files change.

---

### Task 1: Refactor `pendingShell` → `pendingInject` with unified entries

Replace `shellEntry` with a label-based `injectEntry` so both shell and attach entries share one fold formatter. The existing shell entries are rewritten in-place to use the new shape; the fold output for shell-only sessions is byte-identical to before.

**Files:**
- Modify: `cc-agent/internal/transport/shell.go`
- Modify: `cc-agent/internal/transport/shell_test.go`

- [ ] **Step 1: Update existing shell tests to use the new names**

Rewrite `cc-agent/internal/transport/shell_test.go`'s buffer + fold tests to refer to `pendingInject`, `pushShell`, and `foldInject`. The behavior asserted by the existing assertions does not change (the fold output for shell-only sessions stays byte-identical). Replace these four test functions in-place — `TestPendingShell_PushTake` → `TestPendingInject_PushTake`, `TestFoldShellContext_NoEntriesReturnsRawInput` → `TestFoldInject_NoEntriesReturnsRawInput`, `TestFoldShellContext_OneEntry` → `TestFoldInject_OneShellEntry`, `TestFoldShellContext_MultipleEntriesAndEmptyOutput` → `TestFoldInject_MultipleShellEntriesAndEmptyOutput`:

```go
func TestPendingInject_PushTake(t *testing.T) {
	var p pendingInject
	if !p.empty() {
		t.Fatal("new buffer should be empty")
	}
	p.pushShell("echo a", "a\n")
	p.pushShell("echo b", "b\n")
	if p.empty() {
		t.Fatal("buffer should be non-empty")
	}
	got := p.take()
	if len(got) != 2 || got[0].head != "$ echo a" || got[1].head != "$ echo b" {
		t.Errorf("unexpected drain: %+v", got)
	}
	if !p.empty() {
		t.Fatal("take() should drain the buffer")
	}
}

func TestFoldInject_NoEntriesReturnsRawInput(t *testing.T) {
	got := foldInject(nil, "hello")
	if got != "hello" {
		t.Errorf("empty buffer should pass input through; got=%q", got)
	}
}

func TestFoldInject_OneShellEntry(t *testing.T) {
	entries := []injectEntry{{label: "user-shell", head: "$ ls", body: "foo\nbar\n"}}
	got := foldInject(entries, "fix it")
	want := "[user-shell] $ ls\nfoo\nbar\n===\nfix it"
	if got != want {
		t.Errorf("fold mismatch:\nwant=%q\ngot =%q", want, got)
	}
}

func TestFoldInject_MultipleShellEntriesAndEmptyOutput(t *testing.T) {
	entries := []injectEntry{
		{label: "user-shell", head: "$ true", body: ""},
		{label: "user-shell", head: "$ echo y", body: "y\n"},
	}
	got := foldInject(entries, "now what")
	want := "[user-shell] $ true\n(no output)\n---\n[user-shell] $ echo y\ny\n===\nnow what"
	if got != want {
		t.Errorf("fold mismatch:\nwant=%q\ngot =%q", want, got)
	}
}
```

- [ ] **Step 2: Run tests, watch them fail**

Run: `cd cc-agent && go test ./internal/transport/ -run "TestPendingInject|TestFoldInject" -v`
Expected: FAIL — symbols undefined.

- [ ] **Step 3: Rewrite the buffer + fold in `shell.go`**

In `cc-agent/internal/transport/shell.go`, replace everything from the `shellEntry` definition to the end of `foldShellContext` (lines 64-118) with:

```go
// injectEntry is one staged item — either a !! shell capture or an @ file
// attach — waiting to be folded into the next real user message. The label
// drives the fold header (`[user-shell]` or `[user-attach]`); head is the
// post-bracket title line ("$ <cmd>" or "@<path>"); body is the multi-line
// content (command output or file contents, with any markers already baked in
// by the producer).
type injectEntry struct {
	label string // "user-shell" | "user-attach"
	head  string
	body  string
}

// pendingInject is a single-session FIFO of staged !! and @ entries. Not
// goroutine-safe because RunCLI is single-threaded — the REPL goroutine is
// the only writer and reader.
type pendingInject struct {
	entries []injectEntry
}

func (p *pendingInject) pushShell(cmd, out string) {
	p.entries = append(p.entries, injectEntry{label: "user-shell", head: "$ " + cmd, body: out})
}

func (p *pendingInject) pushAttach(path, content string, truncated bool) {
	head := "@" + path
	if truncated {
		head += " (truncated)"
	}
	p.entries = append(p.entries, injectEntry{label: "user-attach", head: head, body: content})
}

func (p *pendingInject) take() []injectEntry {
	out := p.entries
	p.entries = nil
	return out
}

func (p *pendingInject) empty() bool { return len(p.entries) == 0 }

// foldInject renders a drained pendingInject buffer + the operator's fresh
// line into the single user message that gets handed to agent.Run. Shell
// captures and file attaches share one format: a "[label] head" line, then
// the body, with "---" between entries and "===" before the user line.
// When entries is empty the raw line is returned unchanged so a session
// with zero injections is byte-identical to today.
func foldInject(entries []injectEntry, userLine string) string {
	if len(entries) == 0 {
		return userLine
	}
	var sb strings.Builder
	for i, e := range entries {
		if i > 0 {
			sb.WriteString("---\n")
		}
		sb.WriteString("[")
		sb.WriteString(e.label)
		sb.WriteString("] ")
		sb.WriteString(e.head)
		sb.WriteByte('\n')
		body := e.body
		if body == "" {
			body = "(no output)\n"
		} else if !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		sb.WriteString(body)
	}
	sb.WriteString("===\n")
	sb.WriteString(userLine)
	return sb.String()
}
```

- [ ] **Step 4: Update `cli.go` callers (mechanical rename)**

In `cc-agent/internal/transport/cli.go`:

Replace `var bangBuf pendingShell` (line 180) with:

```go
	var injectBuf pendingInject
```

Replace `runBang(ctx, rl, cmd, &bangBuf)` (line 238) with:

```go
			runBang(ctx, rl, cmd, &injectBuf)
```

Replace `_, runErr := ag.Run(runCtx, sessionID, foldShellContext(bangBuf.take(), line))` (line 257) with:

```go
		_, runErr := ag.Run(runCtx, sessionID, foldInject(injectBuf.take(), line))
```

Update `runBang`'s signature (line 592) — replace `buf *pendingShell` with `buf *pendingInject` and the `buf.push(cmd, out)` call (line 611) with `buf.pushShell(cmd, out)`:

```go
func runBang(parent context.Context, rl *readline.Instance, cmd string, buf *pendingInject) {
```

```go
	if buf != nil {
		buf.pushShell(cmd, out)
		fmt.Fprintln(rl.Stdout(), "\033[90m[buffered for next turn]\033[0m")
	}
```

- [ ] **Step 5: Build, run all transport tests**

Run: `cd cc-agent && go build ./... && go test ./internal/transport/ -v`
Expected: PASS — `TestPendingInject*`, `TestFoldInject*`, plus all previously-passing tests (`TestExecShell*`, ESC tests).

- [ ] **Step 6: Commit**

```bash
git add cc-agent/internal/transport/shell.go cc-agent/internal/transport/shell_test.go cc-agent/internal/transport/cli.go
git commit -m "refactor(cc-agent): unify shell/attach buffer as pendingInject"
```

---

### Task 2: `readAttach` helper + tests

`readAttach` resolves a path against the agent cwd, reads the file, rejects directories and binary files, and truncates content above 256 KiB.

**Files:**
- Create: `cc-agent/internal/transport/attach.go`
- Create: `cc-agent/internal/transport/attach_test.go`

- [ ] **Step 1: Write the failing tests**

Create `cc-agent/internal/transport/attach_test.go`:

```go
package transport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadAttach_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, []byte("hi there\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, trunc, err := readAttach(path, dir)
	if err != nil {
		t.Fatalf("readAttach: %v", err)
	}
	if got != "hi there\n" {
		t.Errorf("content mismatch: %q", got)
	}
	if trunc {
		t.Errorf("unexpected truncation")
	}
}

func TestReadAttach_RelativePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rel.txt"), []byte("rel"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _, err := readAttach("rel.txt", dir)
	if err != nil {
		t.Fatalf("readAttach: %v", err)
	}
	if got != "rel" {
		t.Errorf("got %q", got)
	}
}

func TestReadAttach_Missing(t *testing.T) {
	_, _, err := readAttach("/no/such/file/here.txt", "")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadAttach_Directory(t *testing.T) {
	dir := t.TempDir()
	_, _, err := readAttach(dir, "")
	if err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("expected directory error; got %v", err)
	}
}

func TestReadAttach_Binary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bin.dat")
	// 4 KiB of bytes including invalid UTF-8 sequences.
	buf := make([]byte, 4096)
	for i := range buf {
		buf[i] = 0xFE
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := readAttach(path, "")
	if err == nil || !strings.Contains(err.Error(), "binary") {
		t.Fatalf("expected binary-rejection error; got %v", err)
	}
}

func TestReadAttach_Truncation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	big := strings.Repeat("a", 300*1024) // 300 KiB, above the 256 KiB cap
	if err := os.WriteFile(path, []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	got, trunc, err := readAttach(path, "")
	if err != nil {
		t.Fatalf("readAttach: %v", err)
	}
	if !trunc {
		t.Errorf("expected truncated=true")
	}
	if !strings.Contains(got, "…(+") {
		t.Errorf("missing truncation marker: ends with %q", got[len(got)-80:])
	}
}

func TestReadAttach_TildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	path := filepath.Join(home, ".cc-agent-attach-test.txt")
	if err := os.WriteFile(path, []byte("home"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	got, _, err := readAttach("~/.cc-agent-attach-test.txt", "")
	if err != nil {
		t.Fatalf("readAttach with ~: %v", err)
	}
	if got != "home" {
		t.Errorf("got %q", got)
	}
}
```

- [ ] **Step 2: Run tests, watch them fail**

Run: `cd cc-agent && go test ./internal/transport/ -run TestReadAttach -v`
Expected: FAIL — `readAttach` undefined.

- [ ] **Step 3: Implement `readAttach`**

Create `cc-agent/internal/transport/attach.go`:

```go
package transport

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const attachMaxBytes = 256 * 1024

// readAttach resolves path against cwd, reads the file, and returns its
// contents for folding into the next user message. Paths beginning with "~"
// are expanded against the user's home dir; relative paths are joined to
// cwd (the agent's cwd at startup, NOT the operator's pwd, so behavior
// matches what tools see).
//
// Directories are rejected. Files whose first 4 KiB fail a UTF-8 validity
// check are rejected as binary — the LLM can't usefully consume raw bytes.
// Content above attachMaxBytes (256 KiB) is truncated with a "…(+N more
// bytes, truncated)" marker and the truncated flag is set.
func readAttach(path, cwd string) (content string, truncated bool, err error) {
	resolved, err := resolveAttachPath(path, cwd)
	if err != nil {
		return "", false, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", false, fmt.Errorf("stat %s: %w", resolved, err)
	}
	if info.IsDir() {
		return "", false, fmt.Errorf("%s is a directory (use a glob is out of scope; pass a file)", resolved)
	}
	f, err := os.Open(resolved)
	if err != nil {
		return "", false, fmt.Errorf("open %s: %w", resolved, err)
	}
	defer f.Close()

	// Probe the first 4 KiB for UTF-8 validity. utf8.Valid is forgiving:
	// it allows ASCII control bytes (a real text file with \r\n is fine)
	// but flags raw 0xFE/0xFF and other non-UTF-8 sequences typical in
	// compiled binaries.
	probe := make([]byte, 4096)
	n, _ := io.ReadFull(f, probe)
	if !utf8.Valid(probe[:n]) {
		return "", false, fmt.Errorf("%s looks binary (non-UTF-8 in first 4 KiB); use !!base64 if you really need to inject bytes", resolved)
	}
	// Seek back to read everything from start, capped at attachMaxBytes+1
	// so we can detect overflow without reading the entire huge file.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", false, fmt.Errorf("seek %s: %w", resolved, err)
	}
	buf := make([]byte, attachMaxBytes+1)
	total, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", false, fmt.Errorf("read %s: %w", resolved, err)
	}
	if total > attachMaxBytes {
		truncated = true
		remaining := info.Size() - int64(attachMaxBytes)
		return string(buf[:attachMaxBytes]) + fmt.Sprintf("\n…(+%d more bytes, truncated)\n", remaining), true, nil
	}
	return string(buf[:total]), false, nil
}

func resolveAttachPath(path, cwd string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve ~: %w", err)
		}
		path = filepath.Join(home, path[1:])
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	if cwd == "" {
		// fall back to process cwd
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve relative path: %w", err)
		}
	}
	return filepath.Clean(filepath.Join(cwd, path)), nil
}
```

- [ ] **Step 4: Run tests, watch them pass**

Run: `cd cc-agent && go test ./internal/transport/ -run TestReadAttach -v`
Expected: PASS (all 7 cases).

- [ ] **Step 5: Run the full transport suite**

Run: `cd cc-agent && go test ./internal/transport/ -v`
Expected: PASS — no regressions in shell or ESC tests.

- [ ] **Step 6: Commit**

```bash
git add cc-agent/internal/transport/attach.go cc-agent/internal/transport/attach_test.go
git commit -m "feat(cc-agent): readAttach helper for @ file injection"
```

---

### Task 3: Wire `@` into the REPL loop (stage + inline forms)

Detect `@` after `TrimSpace`, decide stage vs inline based on whether there's text after the path, and route either to the inject buffer (stage) or to a one-shot dispatch with the folded content (inline).

**Files:**
- Modify: `cc-agent/internal/transport/cli.go`

- [ ] **Step 1: Add a `runAttach` helper**

Append to `cc-agent/internal/transport/cli.go` (after `runBang`):

```go
// runAttach reads a file and either buffers it for the next turn (when
// userText is empty) or returns a folded "this turn" user message that the
// caller dispatches immediately. cwd is the agent's cwd at REPL startup —
// passed in explicitly so the helper stays test-friendly.
//
// On error, the buffer is left untouched and the error message is written
// to rl.Stderr(); the caller should `continue` the REPL loop.
//
// When userText != "", the returned string is a one-shot folded message
// already containing the file body + "===" + userText. When userText == "",
// the file is pushed to buf and "" is returned.
func runAttach(rl *readline.Instance, path, userText, cwd string, buf *pendingInject) (string, bool) {
	content, truncated, err := readAttach(path, cwd)
	if err != nil {
		fmt.Fprintf(rl.Stderr(), "attach error: %v\n", err)
		return "", false
	}
	size := len(content)
	suffix := fmt.Sprintf("(%d bytes)", size)
	if truncated {
		suffix = fmt.Sprintf("(%d bytes, truncated to %d KiB)", size, attachMaxBytes>>10)
	}
	fmt.Fprintf(rl.Stdout(), "\033[90m@%s %s\033[0m\n", path, suffix)
	if strings.TrimSpace(userText) == "" {
		buf.pushAttach(path, content, truncated)
		fmt.Fprintln(rl.Stdout(), "\033[90m[buffered for next turn]\033[0m")
		return "", false
	}
	// Inline form: build a one-shot entry list (drained existing buffer +
	// this attach) and fold against userText.
	entries := buf.take()
	entries = append(entries, injectEntry{
		label: "user-attach",
		head:  attachHead(path, truncated),
		body:  content,
	})
	return foldInject(entries, userText), true
}

func attachHead(path string, truncated bool) string {
	if truncated {
		return "@" + path + " (truncated)"
	}
	return "@" + path
}
```

- [ ] **Step 2: Add `@` prefix dispatch in the REPL loop**

Locate the `!` block (around line 241-249 of `cli.go`). Immediately after the closing brace of `if strings.HasPrefix(line, "!") { ... }` and BEFORE `runCtx, cancelRun := context.WithCancel(ctx)`, insert:

```go
		if strings.HasPrefix(line, "@") {
			rest := strings.TrimPrefix(line, "@")
			rest = strings.TrimLeft(rest, " \t")
			if rest == "" {
				fmt.Fprintln(rl.Stderr(), "usage: @<path> [prompt]   (attach file; with prompt sends immediately)")
				continue
			}
			path, userText := splitAttachArgs(rest)
			folded, fire := runAttach(rl, path, userText, "", &injectBuf)
			if !fire {
				continue
			}
			// Fall through to a normal turn with the folded content. We
			// rebind `line` so the existing dispatch code below picks it up.
			line = folded
		}
```

Then add the splitter at the bottom of `cli.go`:

```go
// splitAttachArgs splits "<path> <prompt...>" into (path, prompt). The path
// runs up to the first whitespace; everything after the first run of
// whitespace is the prompt. Paths with spaces are not supported in v1.
func splitAttachArgs(s string) (path, prompt string) {
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimSpace(s[i+1:])
}
```

- [ ] **Step 3: Test the splitter**

Append to `cc-agent/internal/transport/shell_test.go` (it's the closest cousin; the splitter is a 4-line pure function so doesn't deserve its own file):

```go
func TestSplitAttachArgs(t *testing.T) {
	cases := []struct {
		in, path, prompt string
	}{
		{"foo.txt", "foo.txt", ""},
		{"foo.txt explain", "foo.txt", "explain"},
		{"foo.txt   what's wrong", "foo.txt", "what's wrong"},
		{"a/b.c\tinline-tab", "a/b.c", "inline-tab"},
	}
	for _, c := range cases {
		gotP, gotT := splitAttachArgs(c.in)
		if gotP != c.path || gotT != c.prompt {
			t.Errorf("split(%q) = (%q, %q); want (%q, %q)", c.in, gotP, gotT, c.path, c.prompt)
		}
	}
}
```

- [ ] **Step 4: Add fold tests for attach-only and mixed shell+attach order**

Append to `cc-agent/internal/transport/shell_test.go`:

```go
func TestFoldInject_AttachOnly(t *testing.T) {
	entries := []injectEntry{
		{label: "user-attach", head: "@/tmp/foo.txt", body: "hello\n"},
	}
	got := foldInject(entries, "summarize")
	want := "[user-attach] @/tmp/foo.txt\nhello\n===\nsummarize"
	if got != want {
		t.Errorf("fold mismatch:\nwant=%q\ngot =%q", want, got)
	}
}

func TestFoldInject_MixedShellAndAttach(t *testing.T) {
	entries := []injectEntry{
		{label: "user-shell", head: "$ ls", body: "a\nb\n"},
		{label: "user-attach", head: "@/tmp/x.log", body: "ERROR: boom\n"},
	}
	got := foldInject(entries, "explain")
	want := "[user-shell] $ ls\na\nb\n---\n[user-attach] @/tmp/x.log\nERROR: boom\n===\nexplain"
	if got != want {
		t.Errorf("fold mismatch:\nwant=%q\ngot =%q", want, got)
	}
}
```

- [ ] **Step 5: Build and run tests**

Run: `cd cc-agent && go build ./... && go test ./internal/transport/ -v`
Expected: PASS — including the new `TestSplitAttachArgs` and `TestFoldInject_MixedShellAndAttach`.

- [ ] **Step 6: Commit**

```bash
git add cc-agent/internal/transport/cli.go cc-agent/internal/transport/shell_test.go
git commit -m "feat(cc-agent): wire @ prefix for file attach in REPL"
```

---

### Task 4: `Agent.RunWithForcedSkill` (router bypass)

A new entry point that runs one turn pinned to a caller-supplied skill, skipping `Router.Route` entirely. The wrapped user message is byte-identical to what a router-picked turn produces, so memory and prefix caches behave the same.

**Files:**
- Modify: `cc-agent/internal/agent/loop.go`
- Modify: `cc-agent/internal/agent/loop_test.go`

- [ ] **Step 1: Refactor the routing block into a helper**

In `cc-agent/internal/agent/loop.go`, replace lines 207-233 (the entire routing comment + `if a.router != nil && a.skills != nil { ... }` block) with a single call:

```go
	persistedInput := a.applyRouter(ctx, userInput, emit)
```

Then add this method below `RunWithListener` (above `runTool`):

```go
// applyRouter runs the auto-router (if configured) and returns the user
// input wrapped with the picked skill's prompt, or the original input when
// no router is configured or no skill matched. emit may surface router
// errors as agent events.
//
// We must NEVER touch a.opts.SystemPrompt here; mutating req.System would
// invalidate the provider's prefix cache.
func (a *Agent) applyRouter(ctx context.Context, userInput string, emit func(Event)) string {
	if a.router == nil || a.skills == nil {
		return userInput
	}
	all := a.skillsSlice()
	if len(all) == 0 {
		return userInput
	}
	name, err := a.router.Route(ctx, userInput, all)
	switch {
	case err != nil:
		emit(Event{Kind: EventError, Text: fmt.Sprintf("router: %v (continuing without skill)", err)})
		return userInput
	case name == "":
		return userInput
	}
	sk, ok := a.skills.Get(name)
	if !ok {
		return userInput
	}
	wrapped := skills.WrapUserInput(userInput, sk)
	if wrapped == userInput {
		return userInput
	}
	emit(Event{Kind: EventRouter, Text: name})
	return wrapped
}
```

- [ ] **Step 2: Build, run existing agent tests**

Run: `cd cc-agent && go build ./... && go test ./internal/agent/ -v`
Expected: PASS — refactor is behavior-preserving.

- [ ] **Step 3: Write the failing test for `RunWithForcedSkill`**

Append to `cc-agent/internal/agent/loop_test.go`:

```go
func TestAgent_RunWithForcedSkill_BypassesRouter(t *testing.T) {
	ctx := context.Background()
	mem := memory.NewSQLite(t.TempDir() + "/m.db")
	defer mem.Close()
	if err := mem.Init(ctx); err != nil {
		t.Fatal(err)
	}
	// Provider records the request and returns a trivial reply.
	var lastUser string
	prov := &fakeProvider{
		complete: func(ctx context.Context, req llm.Request) (llm.Response, error) {
			for _, m := range req.Messages {
				if m.Role == llm.RoleUser {
					lastUser = m.Content
				}
			}
			return llm.Response{
				Message:    llm.Message{Role: llm.RoleAssistant, Content: "ok"},
				StopReason: llm.StopEnd,
			}, nil
		},
	}
	reg := skills.NewRegistry()
	pinned := &skills.Skill{
		Name: "kernel-probe", Description: "probe kernel",
		Prompt: "PROBE-PROMPT",
	}
	reg.Add(pinned)
	// Router that would pick a DIFFERENT skill if invoked — we assert it is
	// NOT invoked.
	routed := false
	router := skills.RouterFunc(func(_ context.Context, _ string, _ []*skills.Skill) (string, error) {
		routed = true
		return "wrong-skill", nil
	})

	ag := agent.New(prov, tools.NewRegistry(), mem, agent.Options{Model: "x", MaxTokens: 8})
	ag.SetSkills(reg, t.TempDir(), nil)
	ag.SetRouter(router)

	var events []agent.Event
	listener := func(e agent.Event) { events = append(events, e) }

	if _, err := ag.RunWithForcedSkill(ctx, "s1", "hello", pinned, listener); err != nil {
		t.Fatalf("RunWithForcedSkill: %v", err)
	}
	if routed {
		t.Error("router should NOT have been invoked under RunWithForcedSkill")
	}
	if !strings.Contains(lastUser, "PROBE-PROMPT") {
		t.Errorf("expected pinned skill prompt to be folded into user msg; got %q", lastUser)
	}
	if !strings.Contains(lastUser, "kernel-probe") {
		t.Errorf("expected pinned skill name in wrap; got %q", lastUser)
	}
	// Confirm we emitted the (pinned) router event.
	foundEvent := false
	for _, e := range events {
		if e.Kind == agent.EventRouter && strings.Contains(e.Text, "kernel-probe") && strings.Contains(e.Text, "pinned") {
			foundEvent = true
		}
	}
	if !foundEvent {
		t.Errorf("expected EventRouter with pinned marker; got events=%+v", events)
	}
}
```

The test uses two helpers that may not yet exist in `loop_test.go`. If `fakeProvider` is missing, define it at the top of the file:

```go
type fakeProvider struct {
	complete func(ctx context.Context, req llm.Request) (llm.Response, error)
}

func (f *fakeProvider) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	return f.complete(ctx, req)
}
```

If `skills.RouterFunc` adapter is missing, add it to `cc-agent/internal/skills/route.go`:

```go
// RouterFunc lets a bare function satisfy Router; useful in tests.
type RouterFunc func(ctx context.Context, userInput string, available []*Skill) (string, error)

func (f RouterFunc) Route(ctx context.Context, userInput string, available []*Skill) (string, error) {
	return f(ctx, userInput, available)
}
```

Confirm before adding either — they may already exist.

Run: `cd cc-agent && grep -n "fakeProvider" internal/agent/loop_test.go`
Run: `cd cc-agent && grep -n "RouterFunc" internal/skills/route.go`

Add only what's missing.

- [ ] **Step 4: Run, watch fail**

Run: `cd cc-agent && go test ./internal/agent/ -run TestAgent_RunWithForcedSkill -v`
Expected: FAIL — `ag.RunWithForcedSkill` undefined.

- [ ] **Step 5: Implement `RunWithForcedSkill`**

In `cc-agent/internal/agent/loop.go`, add immediately after `RunWithListener`:

```go
// RunWithForcedSkill drives one turn pinned to skill — bypassing the
// auto-router. The wrapped user message is byte-identical to what a
// router-picked turn would produce (same skills.WrapUserInput), so memory
// replay and prefix caches behave the same.
//
// A nil skill is equivalent to RunWithListener (no wrap, no route).
//
// Surfaces a "pinned" EventRouter so transcripts make clear the skill was
// operator-chosen, not auto-picked.
func (a *Agent) RunWithForcedSkill(ctx context.Context, sessionID, userInput string, skill *skills.Skill, listener EventListener) (string, error) {
	if skill == nil {
		return a.RunWithListener(ctx, sessionID, userInput, listener)
	}
	emit := func(e Event) {
		if listener != nil {
			listener(e)
		}
	}
	if err := a.ensureSession(ctx, sessionID); err != nil {
		return "", err
	}
	persistedInput := skills.WrapUserInput(userInput, skill)
	if persistedInput != userInput {
		emit(Event{Kind: EventRouter, Text: skill.Name + " (pinned)"})
	}
	return a.runTurnBody(ctx, sessionID, persistedInput, emit)
}
```

This calls a not-yet-extracted `runTurnBody`. Extract it now: in `RunWithListener`, replace everything from the `userMsg := memory.Message{...}` line (current line 234) through the closing `}` of the function (current line 326) with:

```go
	return a.runTurnBody(ctx, sessionID, persistedInput, emit)
}
```

Then add `runTurnBody` immediately below `RunWithForcedSkill` (and above `runTool`). The body is the verbatim contents of the old in-line block, with one signature change: the `emit` closure that previously captured `listener` from `RunWithListener`'s scope is now received as a parameter — behavior is identical.

```go
// runTurnBody is the post-routing body shared by RunWithListener and
// RunWithForcedSkill: persist the user message, then loop tool calls until
// the model stops or MaxIterations is hit.
func (a *Agent) runTurnBody(ctx context.Context, sessionID, persistedInput string, emit func(Event)) (string, error) {
	userMsg := memory.Message{
		Role: string(llm.RoleUser), Content: persistedInput, At: time.Now().UnixMilli(),
	}
	if err := a.mem.AppendMessage(ctx, sessionID, userMsg); err != nil {
		return "", err
	}
	emit(Event{Kind: EventUser, Text: persistedInput})

	defs := toolDefs(a.tools)
	final := ""

	for i := 0; i < a.opts.MaxIterations; i++ {
		hist, err := a.mem.LoadMessages(ctx, sessionID)
		if err != nil {
			return "", err
		}
		req := llm.Request{
			System:    a.opts.SystemPrompt,
			Messages:  toLLM(hist),
			Tools:     defs,
			Model:     a.opts.Model,
			MaxTokens: a.opts.MaxTokens,
		}
		resp, err := a.provider.Complete(ctx, req)
		if err != nil {
			emit(Event{Kind: EventError, Text: err.Error()})
			return "", err
		}
		assistant := memory.FromLLM(resp.Message)
		assistant.At = time.Now().UnixMilli()
		if err := a.mem.AppendMessage(ctx, sessionID, assistant); err != nil {
			return "", err
		}
		if resp.Message.Content != "" {
			emit(Event{Kind: EventAssistant, Text: resp.Message.Content})
			final = resp.Message.Content
		}
		if resp.StopReason != llm.StopToolUse || len(resp.Message.ToolCalls) == 0 {
			return final, nil
		}
		// Once an assistant message with tool_calls is in memory, every call
		// MUST get a tool result message before we exit this iteration —
		// otherwise the next provider request sees malformed history and
		// OpenAI rejects with "tool_calls must be followed by tool messages".
		// Track outstanding ones; flush synthetic results for any left over.
		pending := make(map[string]bool, len(resp.Message.ToolCalls))
		for _, tc := range resp.Message.ToolCalls {
			pending[tc.ID] = true
		}
		flushPending := func(reason string) {
			for id := range pending {
				result := memory.Message{
					Role:       string(llm.RoleTool),
					ToolUseID:  id,
					ToolResult: reason,
					ToolError:  true,
					At:         time.Now().UnixMilli(),
				}
				// Detached context so a canceled run still records the
				// result. The assistant message is already persisted; this
				// keeps the pair intact.
				_ = a.mem.AppendMessage(context.Background(), sessionID, result)
				emit(Event{Kind: EventToolResult, ToolID: id, Text: reason, IsError: true})
				delete(pending, id)
			}
		}
		for _, tc := range resp.Message.ToolCalls {
			emit(Event{Kind: EventToolCall, ToolName: tc.Name, ToolInput: tc.Input, ToolID: tc.ID})
			out, isErr := a.runTool(ctx, tc)
			result := memory.Message{
				Role:       string(llm.RoleTool),
				ToolUseID:  tc.ID,
				ToolResult: out,
				ToolError:  isErr,
				At:         time.Now().UnixMilli(),
			}
			// Detached context: ctx may be canceled, but the tool already
			// produced its result and we owe it to the assistant turn to
			// record one.
			if err := a.mem.AppendMessage(context.Background(), sessionID, result); err != nil {
				flushPending(fmt.Sprintf("memory write failed: %v", err))
				return final, err
			}
			delete(pending, tc.ID)
			emit(Event{Kind: EventToolResult, ToolID: tc.ID, Text: out, IsError: isErr})
			if ctx.Err() != nil {
				flushPending(fmt.Sprintf("canceled by operator: %v", ctx.Err()))
				return final, ctx.Err()
			}
		}
	}
	return final, fmt.Errorf("max iterations reached (%d)", a.opts.MaxIterations)
}
```

- [ ] **Step 6: Build, run all agent tests**

Run: `cd cc-agent && go build ./... && go test ./internal/agent/ -v`
Expected: PASS — including the new `TestAgent_RunWithForcedSkill_BypassesRouter`.

- [ ] **Step 7: Commit**

```bash
git add cc-agent/internal/agent/loop.go cc-agent/internal/agent/loop_test.go cc-agent/internal/skills/route.go
git commit -m "feat(cc-agent): Agent.RunWithForcedSkill bypasses the router"
```

---

### Task 5: `:use <skill>` slash command and REPL plumbing

Wire `nextSkill` REPL state, the slash-command handler, the completer entry, and converge user-turn dispatch through a single helper that picks `Run` or `RunWithForcedSkill`.

**Files:**
- Modify: `cc-agent/internal/transport/cli.go`

- [ ] **Step 1: Add `nextSkill` state to the REPL loop**

In `RunCLI` (after `var injectBuf pendingInject`, ~line 180), add:

```go
	var nextSkill *skills.Skill
```

- [ ] **Step 2: Converge dispatch into a single call site**

The REPL currently calls `ag.Run(...)` directly. Replace the line:

```go
		_, runErr := ag.Run(runCtx, sessionID, foldInject(injectBuf.take(), line))
```

with:

```go
		folded := foldInject(injectBuf.take(), line)
		var runErr error
		if nextSkill != nil {
			fmt.Fprintf(rl.Stdout(), "\033[90m[skill pinned: %s]\033[0m\n", nextSkill.Name)
			_, runErr = ag.RunWithForcedSkill(runCtx, sessionID, folded, nextSkill, nil)
			nextSkill = nil
		} else {
			_, runErr = ag.Run(runCtx, sessionID, folded)
		}
```

- [ ] **Step 3: Pass `&nextSkill` into `handleSlashCommand`**

`:use` mutates REPL state, so we need a way for the slash handler to set `nextSkill`. Change the signature of `handleSlashCommand` to accept a `*(*skills.Skill)` (a pointer to the pointer slot):

In `cli.go`, change:

```go
func handleSlashCommand(ctx context.Context, ag *agent.Agent, rc *skills.RegistryClient, sessionID, line string, approver *CLIApprover) error {
```

to:

```go
func handleSlashCommand(ctx context.Context, ag *agent.Agent, rc *skills.RegistryClient, sessionID, line string, approver *CLIApprover, nextSkill **skills.Skill) error {
```

And update the call site in `RunCLI`:

```go
		if strings.HasPrefix(line, ":") {
			if err := handleSlashCommand(ctx, ag, rc, sessionID, line, approver, &nextSkill); err != nil {
```

- [ ] **Step 4: Implement `:use`**

In `handleSlashCommand`, add a new `case ":use":` (between `:rewind` and `:registry` is fine, but anywhere in the switch works). Insert just before `case ":registry":`:

```go
	case ":use":
		if len(parts) == 1 {
			if *nextSkill == nil {
				fmt.Println("(no skill pinned)")
			} else {
				fmt.Printf("next turn pinned to skill: %s\n", (*nextSkill).Name)
			}
			return nil
		}
		if parts[1] == "-" {
			*nextSkill = nil
			fmt.Println("\033[32m✓\033[0m skill pin cleared")
			return nil
		}
		reg := ag.Skills()
		if reg == nil {
			return fmt.Errorf(":use: skills registry not configured")
		}
		name := parts[1]
		sk, ok := reg.Get(name)
		if !ok {
			return fmt.Errorf("no skill %q (try :skills to list)", name)
		}
		*nextSkill = sk
		if len(parts) > 2 {
			// Combined form: stash skill, then dispatch the trailing prompt
			// immediately. We piggy-back the REPL loop's natural drain by
			// returning a synthetic "sentinel" error that the caller
			// distinguishes — no, that's ugly. Instead, do the dispatch
			// inline here.
			prompt := strings.Join(parts[2:], " ")
			runCtx, cancelRun := context.WithCancel(ctx)
			defer cancelRun()
			fmt.Fprintf(os.Stdout, "\033[90m[skill pinned: %s]\033[0m\n", sk.Name)
			_, err := ag.RunWithForcedSkill(runCtx, sessionID, prompt, sk, nil)
			*nextSkill = nil
			return err
		}
		fmt.Printf("\033[32m✓\033[0m next turn pinned to skill: %s\n", sk.Name)
		return nil
```

Note: the combined form bypasses the inject buffer drain. If the operator typed `@file` and then `:use foo prompt`, the `@file` content is NOT folded in. This is a v1 limitation; the simpler workflow is `:use foo` first, then `@file prompt` (the latter goes through the normal dispatch path which DOES drain the buffer). Document this in the spec's edge cases if not already there — re-check at the end.

- [ ] **Step 5: Add `:use` to the completer**

In `makeCompleter`, add a new `readline.PcItem` for `:use` with the same `localNames` dynamic completion `:reflect` uses. Insert after `readline.PcItem(":rewind"),`:

```go
		readline.PcItem(":use", readline.PcItemDynamic(localNames)),
```

- [ ] **Step 6: Build, run all tests**

Run: `cd cc-agent && go build ./... && go test ./...`
Expected: PASS — no regressions.

- [ ] **Step 7: Commit**

```bash
git add cc-agent/internal/transport/cli.go
git commit -m "feat(cc-agent): :use <skill> one-shot router override"
```

---

### Task 6: Update `:help` text

Document `@` and `:use` in the help block.

**Files:**
- Modify: `cc-agent/internal/transport/cli.go`

- [ ] **Step 1: Locate the help block**

In `cli.go`, find the `case ":help", ":?":` block (now around line 311). The current body lists `! <command>` and `!! <command>`.

- [ ] **Step 2: Add `@` and `:use` lines**

Replace the existing help block with:

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
  :use <skill> [prompt]            force a specific skill on the next turn
  :use                             show the pinned skill
  :use -                           clear the pinned skill
  :quit | :exit | exit | quit | Ctrl+D   leave the REPL
  ! <command>                      run a shell command (LLM does NOT see it)
  !! <command>                     run a shell command and fold (cmd, output)
                                   into the NEXT user message
                                   (each ! is a fresh subshell; cd / interactive
                                   commands like vim, less won't persist or work)
  @<path>                          attach a file to the next user turn
  @<path> <text>                   attach a file and send <text> as the prompt now
  ESC during a run                 cancel the current turn (or the current !)`)
		return nil
```

- [ ] **Step 3: Build, run all tests**

Run: `cd cc-agent && go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add cc-agent/internal/transport/cli.go
git commit -m "docs(cc-agent): document @ and :use in :help"
```

---

### Task 7: Manual smoke test

End-to-end verification in a real terminal. No new code.

**Files:** (none modified)

- [ ] **Step 1: Build the binary**

Run: `cd cc-agent && go build -o /tmp/cc-agent ./cmd/cc-agent`
Expected: build succeeds.

- [ ] **Step 2: Prepare a tiny demo file**

Run:
```bash
mkdir -p /tmp/cc-agent-smoke
echo 'ERROR: connection refused' > /tmp/cc-agent-smoke/log.txt
```

- [ ] **Step 3: Launch the REPL (API key needed for any LLM-dependent steps)**

Run: `/tmp/cc-agent -deny-destructive -memory /tmp/cc-agent-smoke/m.db`

Expected: prompt `you>` appears.

- [ ] **Step 4: Test `@` stage form**

Type at the prompt: `@/tmp/cc-agent-smoke/log.txt`
Expected: dim line `@/tmp/cc-agent-smoke/log.txt (27 bytes)`, then `[buffered for next turn]`. Prompt returns.

Type: `what error did I just attach?`
Expected: the assistant's reply references "connection refused", proving the fold worked.

Inspect the memory write:

```bash
sqlite3 /tmp/cc-agent-smoke/m.db "SELECT content FROM messages WHERE role='user' ORDER BY id DESC LIMIT 1;"
```

Expected: content starts with `[user-attach] @/tmp/cc-agent-smoke/log.txt` and ends with `what error did I just attach?`.

- [ ] **Step 5: Test `@` inline form**

Type: `@/tmp/cc-agent-smoke/log.txt summarize`
Expected: same `@... (N bytes)` echo, then a turn fires immediately. No `[buffered]` marker.

- [ ] **Step 6: Test `@` missing file**

Type: `@/no/such/file.txt`
Expected: `attach error: stat /no/such/file.txt: ...` (no buffer push, prompt returns).

- [ ] **Step 7: Test `@` directory rejection**

Type: `@/tmp/cc-agent-smoke`
Expected: `attach error: ... is a directory ...`.

- [ ] **Step 8: Test `@` binary file rejection**

Run in another shell: `head -c 4096 /bin/ls > /tmp/cc-agent-smoke/bin.dat`
Then type: `@/tmp/cc-agent-smoke/bin.dat`
Expected: `attach error: ... looks binary ...`.

- [ ] **Step 9: Test `:use` with a known skill**

Type: `:skills`
Expected: list of loaded skills. Pick one — say `kernel-probe` for this example.

Type: `:use kernel-probe`
Expected: `✓ next turn pinned to skill: kernel-probe`.

Type: `what's the kernel version?`
Expected: dim `[skill pinned: kernel-probe]`, then the turn runs. Inspect memory:

```bash
sqlite3 /tmp/cc-agent-smoke/m.db "SELECT content FROM messages WHERE role='user' ORDER BY id DESC LIMIT 1;"
```

Expected: content begins with `<skill name="kernel-probe">…</skill>`. After this turn, `:use` (no args) should show `(no skill pinned)`.

- [ ] **Step 10: Test `:use` combined form**

Type: `:use kernel-probe describe the boot device`
Expected: turn fires immediately with the skill, no separate confirmation echo.

- [ ] **Step 11: Test `:use` unknown name**

Type: `:use nonexistent-skill`
Expected: `command error: no skill "nonexistent-skill" (try :skills to list)`. Pin unchanged.

- [ ] **Step 12: Test `:use -` clear**

Type: `:use kernel-probe`
Then: `:use -`
Expected: `✓ skill pin cleared`. Subsequent `:use` (no args) shows `(no skill pinned)`.

- [ ] **Step 13: Test combined `@` + `:use`**

Type: `:use kernel-probe`
Type: `@/tmp/cc-agent-smoke/log.txt explain through the kernel-probe lens`
Expected: the file attaches and the turn fires immediately, **with** the skill wrap applied. Inspect memory:

```bash
sqlite3 /tmp/cc-agent-smoke/m.db "SELECT content FROM messages WHERE role='user' ORDER BY id DESC LIMIT 1;"
```

Expected: content begins with `<skill name="kernel-probe">…</skill>` and contains the `[user-attach]` block.

- [ ] **Step 14: Test `:help` lists everything new**

Type: `:help`
Expected: printout includes `@<path>`, `@<path> <text>`, `:use <skill> [prompt]`, `:use`, `:use -`.

- [ ] **Step 15: Test `:use` tab completion**

Type: `:use ` then press Tab.
Expected: cycles through local skill names.

- [ ] **Step 16: Exit cleanly**

Type: `:quit`
Expected: `bye.`, exit 0.

No commit for this task.

---

## Notes for the implementer

- **Do not modify `memory.Store`, the skill JSON schema, or any tool.** All new state is ephemeral REPL state (`injectBuf`, `nextSkill`) inside `RunCLI`, plus one pure-Go addition (`RunWithForcedSkill`) to `agent.Agent`.
- **The combined `:use <skill> <prompt>` form bypasses the inject buffer drain.** The spec's edge cases doc this; do not try to "fix" it by draining inside `handleSlashCommand` — the buffer is owned by `RunCLI`, not the slash handler. If the operator wants `@file` + `:use foo prompt` they pin first (`:use foo`), then type `@file prompt`, which goes through the converged dispatch path.
- **`readAttach` uses the agent's cwd, not the operator's pwd.** Pass `""` for `cwd` in v1 so it falls back to the process cwd (matches today's tool behavior). If a future change adds `cfg.Cwd` plumbing, pipe it through then — separate change.
- **Binary detection is UTF-8 validity on the first 4 KiB.** This is a heuristic, not a sniff for magic bytes; rare but possible false positives (UTF-16 source files, valid-UTF-8 binaries). Documented.
- **The 256 KiB cap is hard.** Files larger than this are truncated with a marker; the cap is not configurable in v1. If demand emerges, add `-attach-max-bytes` later.
- **`nextSkill` is intentionally one-shot.** Do not add a "sticky pin" mode — see spec rationale.
- **`RunWithForcedSkill` with a nil skill MUST delegate to `RunWithListener`.** Tests assume this; the slash dispatch relies on it as a "no router override" safety valve.
- **No unit tests for `:use` dispatch.** The existing slash commands (`:reflect`, `:install`, `:publish`, etc.) write to `fmt.Println` and aren't unit-tested individually; we follow that convention. Task 7 covers `:use` end-to-end. If a future change extracts slash bodies behind an `io.Writer`, retrofit then.
