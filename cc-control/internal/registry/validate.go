package registry

import (
	"fmt"
	"regexp"
)

const (
	maxPromptBytes  = 32 * 1024
	maxExampleBytes = 1024
	maxExamples     = 10
)

var nameRE = regexp.MustCompile(`^[a-z][a-z0-9-]{1,63}$`)

type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid %s: %s", e.Field, e.Reason)
}

// Validate checks a Skill for publishability against known tool names.
// Returns nil on success, *ValidationError on the first violation.
func Validate(s *Skill, knownTools []string) error {
	if !nameRE.MatchString(s.Name) {
		return &ValidationError{Field: "name", Reason: "must match ^[a-z][a-z0-9-]{1,63}$"}
	}
	if s.Prompt == "" {
		return &ValidationError{Field: "prompt", Reason: "must be non-empty"}
	}
	if len(s.Prompt) > maxPromptBytes {
		return &ValidationError{Field: "prompt", Reason: fmt.Sprintf("exceeds %d bytes", maxPromptBytes)}
	}
	known := map[string]bool{}
	for _, t := range knownTools {
		known[t] = true
	}
	for _, t := range s.Tools {
		if !known[t] {
			return &ValidationError{Field: "tools", Reason: fmt.Sprintf("unknown tool %q", t)}
		}
	}
	if len(s.Examples) > maxExamples {
		return &ValidationError{Field: "examples", Reason: fmt.Sprintf("more than %d examples", maxExamples)}
	}
	for i, e := range s.Examples {
		if len(e) > maxExampleBytes {
			return &ValidationError{Field: "examples", Reason: fmt.Sprintf("example %d exceeds %d bytes", i, maxExampleBytes)}
		}
	}
	return nil
}
