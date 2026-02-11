package core

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// stripTermEscapes
// ---------------------------------------------------------------------------

func TestStripTermEscapes_CSI(t *testing.T) {
	// CSI colour + cursor movement
	raw := []byte("\x1b[31mhello\x1b[0m world\x1b[2J")
	got := stripTermEscapes(raw)
	if got != "hello world" {
		t.Fatalf("CSI strip: got %q", got)
	}
}

func TestStripTermEscapes_CSI_CursorForward(t *testing.T) {
	// CSI C (cursor forward) should become a space — Claude Code uses it between words
	raw := []byte("Esc\x1b[1Cto\x1b[1Ccancel\x1b[1C\xc2\xb7\x1b[1CTab\x1b[1Cto\x1b[1Camend")
	got := stripTermEscapes(raw)
	if !strings.Contains(got, "Esc") || !strings.Contains(got, "cancel") || !strings.Contains(got, "amend") {
		t.Fatalf("cursor forward strip: got %q", got)
	}
	// After collapse, words should be separated by spaces
	collapsed := collapseWhitespace(got)
	if !strings.Contains(collapsed, "Esc to cancel") {
		t.Fatalf("cursor forward collapse: got %q", collapsed)
	}
}

func TestStripTermEscapes_CSI_CursorDown(t *testing.T) {
	// CSI B (cursor down) should become a newline
	raw := []byte("line1\x1b[1Bline2")
	got := stripTermEscapes(raw)
	if got != "line1\nline2" {
		t.Fatalf("cursor down: got %q", got)
	}
}

func TestStripTermEscapes_OSC(t *testing.T) {
	// OSC title terminated by BEL
	raw := []byte("\x1b]0;my title\x07visible text")
	got := stripTermEscapes(raw)
	if got != "visible text" {
		t.Fatalf("OSC(BEL) strip: got %q", got)
	}
	// OSC terminated by ST (ESC \)
	raw2 := []byte("\x1b]8;id=x;https://example.com\x1b\\link text\x1b]8;;\x1b\\")
	got2 := stripTermEscapes(raw2)
	if got2 != "link text" {
		t.Fatalf("OSC(ST) strip: got %q", got2)
	}
}

func TestStripTermEscapes_SCS(t *testing.T) {
	// ESC(B is common SCS (select G0 charset = ASCII)
	raw := []byte("\x1b(Bhello\x1b)0world")
	got := stripTermEscapes(raw)
	if got != "helloworld" {
		t.Fatalf("SCS strip: got %q", got)
	}
}

func TestStripTermEscapes_DCS(t *testing.T) {
	raw := []byte("before\x1bPsome dcs payload\x1b\\after")
	got := stripTermEscapes(raw)
	if got != "beforeafter" {
		t.Fatalf("DCS strip: got %q", got)
	}
}

func TestStripTermEscapes_CRNormalization(t *testing.T) {
	raw := []byte("line1\r\nline2\rline3")
	got := stripTermEscapes(raw)
	// \r -> \n, so we get line1\n\nline2\nline3
	if !strings.Contains(got, "line1") || !strings.Contains(got, "line2") || !strings.Contains(got, "line3") {
		t.Fatalf("CR norm: got %q", got)
	}
}

func TestStripTermEscapes_TabToSpace(t *testing.T) {
	raw := []byte("a\tb")
	got := stripTermEscapes(raw)
	if got != "a b" {
		t.Fatalf("tab->space: got %q", got)
	}
}

func TestStripTermEscapes_C0Dropped(t *testing.T) {
	// BEL and BS control bytes are dropped (BS does not erase the preceding char;
	// we only strip the control byte itself).
	raw := []byte("he\x07l\x08lo")
	got := stripTermEscapes(raw)
	if got != "hello" {
		t.Fatalf("C0 drop: got %q", got)
	}
}

// ---------------------------------------------------------------------------
// collapseWhitespace
// ---------------------------------------------------------------------------

func TestCollapseWhitespace(t *testing.T) {
	in := "hello    world\n\n\n\nfoo"
	got := collapseWhitespace(in)
	if got != "hello world\n\nfoo" {
		t.Fatalf("collapse: got %q", got)
	}
}

// ---------------------------------------------------------------------------
// PromptDetector.Feed – plain text patterns
// ---------------------------------------------------------------------------

func TestDetector_YN(t *testing.T) {
	d := NewPromptDetector()
	matched, _ := d.Feed("s1", []byte("Continue? (y/n)"))
	if !matched {
		t.Fatal("should match (y/n)")
	}
}

func TestDetector_Confirm(t *testing.T) {
	d := NewPromptDetector()
	matched, _ := d.Feed("s1", []byte("Please confirm the action"))
	if !matched {
		t.Fatal("should match confirm")
	}
}

// ---------------------------------------------------------------------------
// PromptDetector.Feed – Claude Code menu prompt (screenshot scenario)
// ---------------------------------------------------------------------------

func TestDetector_ClaudeCodeCreateFileMenu_Plain(t *testing.T) {
	// Simulates the prompt from the screenshot without escape sequences
	prompt := `Create file
abcdef

 1 (No content)

Do you want to create abcdef?
❯ 1. Yes
  2. Yes, allow all edits during this session (shift+tab)
  3. No

Esc to cancel · Tab to amend`

	d := NewPromptDetector()
	matched, excerpt := d.Feed("s1", []byte(prompt))
	if !matched {
		t.Fatalf("plain menu prompt should match, excerpt=%q", excerpt)
	}
	if !strings.Contains(excerpt, "Do you want to create") {
		t.Fatalf("excerpt should contain prompt question, got %q", excerpt)
	}
}

func TestDetector_ClaudeCodeCreateFileMenu_WithEscapes(t *testing.T) {
	// Same prompt but peppered with real terminal escape sequences:
	// - ESC(B  (SCS)
	// - CSI colour codes
	// - OSC hyperlink
	prompt := "\x1b(B\x1b[m\x1b[38;5;214mCreate file\x1b[0m\r\n" +
		"\x1b]8;id=1;file:///tmp/abcdef\x1b\\abcdef\x1b]8;;\x1b\\\r\n\r\n" +
		" 1 (No content)\r\n\r\n" +
		"\x1b[1mDo you want to create abcdef?\x1b[0m\r\n" +
		"\x1b[32m❯\x1b[0m 1. Yes\r\n" +
		"  2. Yes, allow all edits during this session (shift+tab)\r\n" +
		"  3. No\r\n\r\n" +
		"\x1b[2mEsc to cancel · Tab to amend\x1b[0m"

	d := NewPromptDetector()
	matched, excerpt := d.Feed("s1", []byte(prompt))
	if !matched {
		t.Fatalf("escaped menu prompt should match, excerpt=%q", excerpt)
	}
}

func TestDetector_EscToCancelTabToAmend_WithEscapes(t *testing.T) {
	// Just the tail of a prompt with heavy escape sequences
	prompt := "\x1b[2m\x1b(BEsc to cancel\x1b[0m \x1b[2m·\x1b[0m \x1b[2mTab to amend\x1b[0m"
	d := NewPromptDetector()
	matched, _ := d.Feed("s1", []byte(prompt))
	if !matched {
		t.Fatal("Esc to cancel / Tab to amend should match even through escapes")
	}
}

func TestDetector_RealClaudeCodePTYChunk(t *testing.T) {
	// Simulates an actual PTY chunk from Claude Code where CSI 1 C (cursor forward)
	// is used as spacing between every word.
	chunk := "estfile123?\r\r\n" +
		"\x1b[1C\xe2\x9d\xaf\x1b[1C1.\x1b[1CYes\r\r\n" +
		"\x1b[3C2.\x1b[1CYes,\x1b[1Callow\x1b[1Call\x1b[1Cedits\x1b[1Cduring\x1b[1Cthis\x1b[1Csession\x1b[1C(shift+tab)\r\r\n" +
		"\x1b[3C3.\x1b[1CNo\r\r\n\r\r\n" +
		"\x1b[1CEsc\x1b[1Cto\x1b[1Ccancel\x1b[1C\xc2\xb7\x1b[1CTab\x1b[1Cto\x1b[1Camend\r\r\n" +
		"\x1b[?2026l"

	d := NewPromptDetector()
	// First feed with the "Do you want to create t" part
	d.Feed("s1", []byte("Do you want to create t"))
	// Then the rest
	matched, excerpt := d.Feed("s1", []byte(chunk))
	if !matched {
		t.Fatalf("real PTY chunk should match, excerpt=%q", excerpt)
	}
}

// ---------------------------------------------------------------------------
// PromptDetector.Feed – "Do you want to <verb>?" standalone trigger
// ---------------------------------------------------------------------------

func TestDetector_DoYouWantToVerb(t *testing.T) {
	d := NewPromptDetector()
	matched, _ := d.Feed("s1", []byte("Do you want to proceed?"))
	if !matched {
		t.Fatal("standalone 'Do you want to proceed?' should match")
	}
}

// ---------------------------------------------------------------------------
// PromptDetector.Clear prevents re-triggering
// ---------------------------------------------------------------------------

func TestDetector_ClearPreventsRetrigger(t *testing.T) {
	d := NewPromptDetector()
	matched, _ := d.Feed("s1", []byte("Continue? (y/n)"))
	if !matched {
		t.Fatal("first feed should match")
	}
	d.Clear("s1")
	// Feed innocuous text; old prompt should not re-trigger
	matched2, _ := d.Feed("s1", []byte("some normal output"))
	if matched2 {
		t.Fatal("after Clear, innocuous text should not match")
	}
}

// ---------------------------------------------------------------------------
// PromptDetector.Feed – should NOT match on regular output
// ---------------------------------------------------------------------------

func TestDetector_NoFalsePositive(t *testing.T) {
	d := NewPromptDetector()
	cases := []string{
		"compiling main.go...",
		"test passed",
		"downloading dependencies",
		"192.168.1.1 - GET /api/health 200",
	}
	for _, c := range cases {
		matched, _ := d.Feed("neg", []byte(c))
		if matched {
			t.Fatalf("false positive on %q", c)
		}
		d.Clear("neg")
	}
}
