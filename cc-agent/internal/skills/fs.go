package skills

import "os"

// Indirected so tests can stub the filesystem if ever needed; default just
// delegates to os.
var (
	mkdirAll = func(dir string) error { return os.MkdirAll(dir, 0o755) }
	writeFile = func(path string, data []byte) error { return os.WriteFile(path, data, 0o644) }
)
