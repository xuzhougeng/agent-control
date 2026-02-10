package security

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func ParseCSV(v string) []string {
	raw := strings.Split(v, ",")
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		s := strings.TrimSpace(r)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func NormalizeRoots(roots []string) ([]string, error) {
	if len(roots) == 0 {
		return nil, errors.New("allow roots required")
	}
	out := make([]string, 0, len(roots))
	for _, root := range roots {
		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		if st, err := os.Stat(abs); err != nil || !st.IsDir() {
			return nil, errors.New("invalid allow root: " + abs)
		}
		real, err := filepath.EvalSymlinks(abs)
		if err != nil {
			real = abs
		}
		out = append(out, filepath.Clean(real))
	}
	return out, nil
}

func ValidateCWD(cwd string, roots []string) error {
	if cwd == "" {
		return errors.New("cwd required")
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return err
	}
	real = filepath.Clean(real)
	for _, root := range roots {
		rel, err := filepath.Rel(root, real)
		if err != nil {
			continue
		}
		if rel == "." || (!strings.HasPrefix(rel, "..") && rel != "..") {
			return nil
		}
	}
	return errors.New("cwd not in allow roots")
}

func FilterEnv(input map[string]string, allowedKeys map[string]struct{}, allowedPrefix string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string)
	for k, v := range input {
		if _, ok := allowedKeys[k]; ok {
			out[k] = v
			continue
		}
		if allowedPrefix != "" && strings.HasPrefix(k, allowedPrefix) {
			out[k] = v
		}
	}
	return out
}
