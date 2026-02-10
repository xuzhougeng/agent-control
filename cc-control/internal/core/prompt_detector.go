package core

import (
	"regexp"
	"strings"
	"sync"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

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
	clean := ansiPattern.ReplaceAllString(string(raw), "")
	if clean == "" {
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
			return true, lastLines(buf, 4)
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
