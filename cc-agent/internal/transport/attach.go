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
		return "", false, err
	}
	if info.IsDir() {
		return "", false, fmt.Errorf("%s is a directory (glob patterns are out of scope; pass a file path)", resolved)
	}
	f, err := os.Open(resolved)
	if err != nil {
		return "", false, err
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
		if remaining <= 0 {
			remaining = 1
		}
		return string(buf[:attachMaxBytes]) + fmt.Sprintf("\n…(+%d more bytes, truncated)\n", remaining), true, nil
	}
	return string(buf[:total]), false, nil
}

func resolveAttachPath(path, cwd string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve ~: %w", err)
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
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
