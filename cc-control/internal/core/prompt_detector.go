package core

import (
	"regexp"
	"strings"
	"sync"
)

// stripTermEscapes removes all common terminal escape / control sequences from
// raw PTY output, producing clean printable text suitable for regex matching.
//
// Handled sequence families:
//   - CSI  : ESC [ ... <final byte 0x40-0x7E>
//   - OSC  : ESC ] ... (BEL | ESC \)
//   - DCS/SOS/PM/APC : ESC P/X/^/_ ... ESC \
//   - SCS  : ESC ( <char>, ESC ) <char>, etc.
//   - Simple two-byte: ESC <0x20-0x2F>* <0x30-0x7E>
//   - C0 control chars (except \n and \t) are dropped; \r normalised to \n;
//     \t replaced by a single space.
func stripTermEscapes(raw []byte) string {
	var b strings.Builder
	b.Grow(len(raw))
	i := 0
	n := len(raw)
	for i < n {
		ch := raw[i]

		// --- ESC-initiated sequences ---
		if ch == 0x1b && i+1 < n {
			next := raw[i+1]
			switch {
			// CSI: ESC [
			case next == '[':
				i += 2
				for i < n && raw[i] < 0x40 {
					i++
				}
				var csiFinal byte
				if i < n {
					csiFinal = raw[i]
					i++ // skip final byte
				}
				// Cursor-forward (C) → space; cursor-down (B) → newline.
				// Claude Code uses CSI 1 C between every word for spacing.
				switch csiFinal {
				case 'C':
					b.WriteByte(' ')
				case 'B':
					b.WriteByte('\n')
				}
				continue
			// OSC: ESC ]  ...  (BEL | ESC \)
			case next == ']':
				i += 2
				for i < n {
					if raw[i] == 0x07 {
						i++
						break
					}
					if raw[i] == 0x1b && i+1 < n && raw[i+1] == '\\' {
						i += 2
						break
					}
					i++
				}
				continue
			// DCS (ESC P), SOS (ESC X), PM (ESC ^), APC (ESC _): terminated by ST (ESC \)
			case next == 'P' || next == 'X' || next == '^' || next == '_':
				i += 2
				for i < n {
					if raw[i] == 0x1b && i+1 < n && raw[i+1] == '\\' {
						i += 2
						break
					}
					i++
				}
				continue
			// SCS: ESC ( <char>, ESC ) <char>, ESC * <char>, ESC + <char>
			case next == '(' || next == ')' || next == '*' || next == '+':
				i += 3
				continue
			default:
				// Generic two-byte (e.g. ESC =, ESC >, ESC M, etc.)
				// Optional intermediate bytes 0x20-0x2F, then final 0x30-0x7E.
				i += 2
				for i < n && raw[i] >= 0x20 && raw[i] <= 0x2F {
					i++
				}
				if i < n && raw[i] >= 0x30 && raw[i] <= 0x7E {
					i++
				}
				continue
			}
		}

		// --- Standalone C1 (0x80-0x9F) treated as CSI/OSC if 0x9B/0x9D ---
		if ch == 0x9B { // C1 CSI
			i++
			for i < n && raw[i] < 0x40 {
				i++
			}
			if i < n {
				i++
			}
			continue
		}
		if ch == 0x9D { // C1 OSC
			i++
			for i < n && raw[i] != 0x07 && raw[i] != 0x9C {
				i++
			}
			if i < n {
				i++
			}
			continue
		}

		// --- C0 control characters ---
		if ch < 0x20 {
			switch ch {
			case '\n':
				b.WriteByte('\n')
			case '\r':
				b.WriteByte('\n')
			case '\t':
				b.WriteByte(' ')
			// drop everything else (BEL, BS, etc.)
			}
			i++
			continue
		}

		// DEL
		if ch == 0x7F {
			i++
			continue
		}

		b.WriteByte(ch)
		i++
	}
	return b.String()
}

// collapseWhitespace reduces runs of spaces/tabs to a single space per line and
// deduplicates consecutive blank lines, producing stable text for regex.
func collapseWhitespace(s string) string {
	spaceRun := regexp.MustCompile(`[^\S\n]+`)
	s = spaceRun.ReplaceAllString(s, " ")
	blankLines := regexp.MustCompile(`\n{3,}`)
	return blankLines.ReplaceAllString(s, "\n\n")
}

type PromptDetector struct {
	mu        sync.Mutex
	buffers   map[string]string
	patterns  []*regexp.Regexp
	maxBuffer int
}

func NewPromptDetector() *PromptDetector {
	rawPatterns := []string{
		`(?i)(approve|reject)`,
		`(?i)\(y/n\)`,
		`(?i)\[y/N\]`,
		`(?i)\bconfirm\b`,
		`(?i)continue\?`,
		// Claude Code / Cursor-style approval menu prompt (examples):
		// - "Do you want to proceed?" / "Do you want to create <file>?"
		// - "1. Yes" ... "3. No"
		// - "Esc to cancel · Tab to amend"
		`(?is)\bdo\s+you\s+want\s+to\b.{0,800}1[.)]\s*[Yy]es\b`,
		`(?is)\besc\s+to\s+cancel\b.{0,300}\btab\s+to\s+amend\b`,
		// Standalone "Do you want to <verb>?" as a single-line trigger
		`(?i)\bdo\s+you\s+want\s+to\s+\w+.*\?`,
	}
	patterns := make([]*regexp.Regexp, 0, len(rawPatterns))
	for _, p := range rawPatterns {
		patterns = append(patterns, regexp.MustCompile(p))
	}
	return &PromptDetector{
		buffers:   make(map[string]string),
		patterns:  patterns,
		maxBuffer: 4096,
	}
}

// Feed returns (matched, excerpt).
func (d *PromptDetector) Feed(sessionID string, raw []byte) (bool, string) {
	clean := collapseWhitespace(stripTermEscapes(raw))
	if strings.TrimSpace(clean) == "" {
		return false, ""
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	buf := d.buffers[sessionID] + clean
	if len(buf) > d.maxBuffer {
		buf = buf[len(buf)-d.maxBuffer:]
	}
	d.buffers[sessionID] = buf

	for _, p := range d.patterns {
		if p.MatchString(buf) {
			return true, lastLines(buf, 12)
		}
	}
	return false, ""
}

func (d *PromptDetector) Clear(sessionID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.buffers, sessionID)
}

func lastLines(s string, count int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= count {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(strings.Join(lines[len(lines)-count:], "\n"))
}
