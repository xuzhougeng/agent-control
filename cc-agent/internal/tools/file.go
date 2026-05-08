package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Read reads a UTF-8 text file with safety bounds and returns its content,
// optionally line-windowed.
type Read struct {
	Cwd          string
	MaxBytes     int
	AllowedRoots []string // optional containment list; empty = anything under Cwd
}

func NewRead(cwd string) *Read {
	return &Read{Cwd: cwd, MaxBytes: 256 * 1024}
}

func (*Read) Name() string { return "read" }

func (*Read) Description() string {
	return "Read a text file from disk. Optionally pass offset/limit (1-indexed line numbers) to read a slice."
}

func (*Read) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":   map[string]any{"type": "string", "description": "File path; absolute or relative to agent cwd."},
			"offset": map[string]any{"type": "integer", "description": "1-indexed start line. Optional."},
			"limit":  map[string]any{"type": "integer", "description": "Max lines to return. Optional, default 2000."},
		},
		"required": []string{"path"},
	}
}

func (r *Read) Run(_ context.Context, input map[string]any) (string, error) {
	p, err := MustString(input, "path")
	if err != nil {
		return "", err
	}
	full, err := r.resolve(p)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	if st.IsDir() {
		return "", fmt.Errorf("path is a directory: %s", p)
	}
	limitBytes := r.MaxBytes
	if limitBytes <= 0 {
		limitBytes = 256 * 1024
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	if len(data) > limitBytes {
		data = append(data[:limitBytes], []byte(fmt.Sprintf("\n... [truncated %d bytes] ...", len(data)-limitBytes))...)
	}
	offset := OptInt(input, "offset", 0)
	limit := OptInt(input, "limit", 2000)
	if offset > 0 || limit > 0 {
		lines := strings.Split(string(data), "\n")
		start := offset - 1
		if start < 0 {
			start = 0
		}
		if start >= len(lines) {
			return "", nil
		}
		end := start + limit
		if limit <= 0 || end > len(lines) {
			end = len(lines)
		}
		return strings.Join(lines[start:end], "\n"), nil
	}
	return string(data), nil
}

func (r *Read) resolve(p string) (string, error) {
	if !filepath.IsAbs(p) {
		p = filepath.Join(r.Cwd, p)
	}
	clean := filepath.Clean(p)
	if len(r.AllowedRoots) > 0 {
		ok := false
		for _, root := range r.AllowedRoots {
			if root == "" {
				continue
			}
			rel, err := filepath.Rel(root, clean)
			if err == nil && !strings.HasPrefix(rel, "..") {
				ok = true
				break
			}
		}
		if !ok {
			return "", fmt.Errorf("path not under any allowed root: %s", clean)
		}
	}
	return clean, nil
}

// Write writes a file. It refuses to silently overwrite unless overwrite=true.
type Write struct {
	Cwd          string
	AllowedRoots []string
}

func NewWrite(cwd string) *Write {
	return &Write{Cwd: cwd}
}

func (*Write) Name() string { return "write" }

func (*Write) Description() string {
	return "Write a file. Set overwrite=true to replace existing content. Creates parent directories on demand."
}

func (*Write) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":      map[string]any{"type": "string"},
			"content":   map[string]any{"type": "string"},
			"overwrite": map[string]any{"type": "boolean"},
		},
		"required": []string{"path", "content"},
	}
}

func (w *Write) Run(_ context.Context, input map[string]any) (string, error) {
	p, err := MustString(input, "path")
	if err != nil {
		return "", err
	}
	content, err := MustString(input, "content")
	if err != nil {
		return "", err
	}
	overwrite, _ := input["overwrite"].(bool)
	full, err := (&Read{Cwd: w.Cwd, AllowedRoots: w.AllowedRoots}).resolve(p)
	if err != nil {
		return "", err
	}
	if !overwrite {
		if _, err := os.Stat(full); err == nil {
			return "", errors.New("file exists; pass overwrite=true to replace")
		}
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(content), full), nil
}
