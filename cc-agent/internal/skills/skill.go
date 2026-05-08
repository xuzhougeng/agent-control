// Package skills holds reusable, named recipes the agent can opt into for a
// session. A skill bundles a system prompt fragment, a tool whitelist, and
// optional examples. Skills are loaded from a directory of YAML/JSON files at
// startup.
//
// MVP scope: load + register only. Skill self-evolution (writing back distilled
// learnings, à la GenericAgent's reflect/) is deferred; the Reflect interface
// is reserved here to keep call sites stable when it lands.
package skills

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Skill struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Prompt      string   `json:"prompt"`        // appended to the base system prompt when active
	Tools       []string `json:"tools"`         // optional tool whitelist; empty = all
	Examples    []string `json:"examples"`
	SourcePath  string   `json:"-"`
}

type Registry struct {
	skills map[string]*Skill
}

func NewRegistry() *Registry {
	return &Registry{skills: map[string]*Skill{}}
}

// LoadDir loads .json files in dir as skill definitions. Subdirectories are
// scanned recursively. Missing dir returns nil (no skills).
func (r *Registry) LoadDir(dir string) error {
	if dir == "" {
		return nil
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(p, ".json") {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		var s Skill
		if err := json.Unmarshal(raw, &s); err != nil {
			return fmt.Errorf("parse %s: %w", p, err)
		}
		if s.Name == "" {
			return fmt.Errorf("skill %s missing name", p)
		}
		s.SourcePath = p
		r.skills[s.Name] = &s
		return nil
	})
}

func (r *Registry) Get(name string) (*Skill, bool) {
	s, ok := r.skills[name]
	return s, ok
}

// Names returns skill names sorted for stable display.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.skills))
	for n := range r.skills {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

