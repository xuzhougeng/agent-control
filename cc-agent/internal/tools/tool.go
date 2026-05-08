// Package tools defines the Tool interface used by the agent loop. Tools are
// pure: given an input map (already validated against InputSchema), they
// return a textual result for the model to read.
package tools

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

type Tool interface {
	Name() string
	Description() string
	InputSchema() map[string]any
	Run(ctx context.Context, input map[string]any) (string, error)
}

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}}
}

func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// All returns tools sorted by name for stable iteration.
func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Tool, 0, len(names))
	for _, n := range names {
		out = append(out, r.tools[n])
	}
	return out
}

// MustString is a small helper for tools to extract a required string field.
func MustString(input map[string]any, key string) (string, error) {
	v, ok := input[key]
	if !ok {
		return "", fmt.Errorf("missing required field %q", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("field %q must be string", key)
	}
	return s, nil
}

// OptString is a helper for optional string fields.
func OptString(input map[string]any, key, def string) string {
	v, ok := input[key]
	if !ok {
		return def
	}
	s, ok := v.(string)
	if !ok {
		return def
	}
	return s
}

// OptInt extracts an optional int from JSON-decoded input (where numbers are
// float64 by default).
func OptInt(input map[string]any, key string, def int) int {
	v, ok := input[key]
	if !ok {
		return def
	}
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	}
	return def
}
