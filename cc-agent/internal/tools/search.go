package tools

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Grep searches files for a regex match. Pure-Go, no shell ripgrep dependency.
type Grep struct {
	Cwd          string
	MaxMatches   int
	MaxFileBytes int64 // skip files larger than this
}

func NewGrep(cwd string) *Grep {
	return &Grep{Cwd: cwd, MaxMatches: 200, MaxFileBytes: 5 * 1024 * 1024}
}

func (*Grep) Name() string { return "grep" }

func (*Grep) Description() string {
	return "Search for a regex pattern in files. Returns matching lines with file:line. " +
		"Use 'path' to scope to a directory or file (default agent cwd)."
}

func (*Grep) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern":      map[string]any{"type": "string", "description": "Go RE2 regex."},
			"path":         map[string]any{"type": "string", "description": "Optional file or directory."},
			"glob":         map[string]any{"type": "string", "description": "Optional filename glob, e.g. '*.go'."},
			"ignore_case":  map[string]any{"type": "boolean"},
			"max_results":  map[string]any{"type": "integer"},
		},
		"required": []string{"pattern"},
	}
}

func (g *Grep) Run(_ context.Context, input map[string]any) (string, error) {
	pattern, err := MustString(input, "pattern")
	if err != nil {
		return "", err
	}
	if ignore, _ := input["ignore_case"].(bool); ignore {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("bad regex: %w", err)
	}
	root := OptString(input, "path", g.Cwd)
	if !filepath.IsAbs(root) {
		root = filepath.Join(g.Cwd, root)
	}
	globPat := OptString(input, "glob", "")
	max := OptInt(input, "max_results", g.MaxMatches)

	matches := 0
	var sb strings.Builder
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" || base == "vendor" {
				return fs.SkipDir
			}
			return nil
		}
		if globPat != "" {
			ok, _ := filepath.Match(globPat, filepath.Base(path))
			if !ok {
				return nil
			}
		}
		info, err := d.Info()
		if err != nil || info.Size() > g.MaxFileBytes {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if re.MatchString(line) {
				fmt.Fprintf(&sb, "%s:%d:%s\n", path, i+1, line)
				matches++
				if matches >= max {
					return fs.SkipAll
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if matches == 0 {
		return "no matches", nil
	}
	return sb.String(), nil
}

// Glob lists files matching a shell glob.
type Glob struct {
	Cwd        string
	MaxResults int
}

func NewGlob(cwd string) *Glob {
	return &Glob{Cwd: cwd, MaxResults: 200}
}

func (*Glob) Name() string { return "glob" }

func (*Glob) Description() string {
	return "List files matching a glob pattern (filepath.Match semantics, applied to basename per file under path)."
}

func (*Glob) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string", "description": "Glob pattern, e.g. '*.go'."},
			"path":    map[string]any{"type": "string", "description": "Search root. Default agent cwd."},
		},
		"required": []string{"pattern"},
	}
}

func (g *Glob) Run(_ context.Context, input map[string]any) (string, error) {
	pat, err := MustString(input, "pattern")
	if err != nil {
		return "", err
	}
	root := OptString(input, "path", g.Cwd)
	if !filepath.IsAbs(root) {
		root = filepath.Join(g.Cwd, root)
	}
	var hits []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return nil
		}
		if d.IsDir() {
			base := filepath.Base(p)
			if base == ".git" || base == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		if ok, _ := filepath.Match(pat, filepath.Base(p)); ok {
			hits = append(hits, p)
			if len(hits) >= g.MaxResults {
				return fs.SkipAll
			}
		}
		return nil
	})
	if len(hits) == 0 {
		return "no matches", nil
	}
	sort.Strings(hits)
	return strings.Join(hits, "\n"), nil
}
