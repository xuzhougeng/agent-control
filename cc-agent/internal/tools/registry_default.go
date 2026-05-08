package tools

// DefaultRegistry returns a registry preloaded with the standard ops toolset.
// Pass an empty allow-list for read/write to permit the entire fs (subject to
// the underlying user permissions); otherwise paths must be under one of the
// listed roots.
func DefaultRegistry(cwd string, allowedRoots []string) *Registry {
	r := NewRegistry()
	r.Register(NewBash(cwd))
	read := NewRead(cwd)
	read.AllowedRoots = allowedRoots
	r.Register(read)
	write := NewWrite(cwd)
	write.AllowedRoots = allowedRoots
	r.Register(write)
	r.Register(NewGrep(cwd))
	r.Register(NewGlob(cwd))
	r.Register(&SysInfo{})
	r.Register(NewProcList())
	r.Register(NewLogTail())
	return r
}
