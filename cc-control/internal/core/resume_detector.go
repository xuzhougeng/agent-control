package core

import (
	"regexp"
	"strings"
	"sync"
)

var resumePattern = regexp.MustCompile(`(?i)\bclaude(?:-code)?\s+--resume\s+([0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12})\b`)


type ResumeDetector struct {
	mu        sync.Mutex
	buffers   map[string]string
	maxBuffer int
}

func NewResumeDetector() *ResumeDetector {
	return &ResumeDetector{
		buffers:   make(map[string]string),
		maxBuffer: 4096,
	}
}

// Feed returns (resumeID, matched).
func (d *ResumeDetector) Feed(sessionID string, raw []byte) (string, bool) {
	clean := stripTermEscapes(raw)
	if clean == "" {
		return "", false
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	buf := d.buffers[sessionID] + clean
	if len(buf) > d.maxBuffer {
		buf = buf[len(buf)-d.maxBuffer:]
	}
	d.buffers[sessionID] = buf

	all := resumePattern.FindAllStringSubmatch(buf, -1)
	if len(all) == 0 {
		return "", false
	}
	last := all[len(all)-1]
	if len(last) < 2 {
		return "", false
	}
	return strings.ToLower(last[1]), true
}

func (d *ResumeDetector) Clear(sessionID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.buffers, sessionID)
}
