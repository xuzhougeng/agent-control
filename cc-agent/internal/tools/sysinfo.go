package tools

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// SysInfo emits a concise snapshot of the host: kernel, uptime, load, memory,
// disk usage. Implementation shells out to common Unix tools; output is best
// effort.
type SysInfo struct{}

func (*SysInfo) Name() string { return "sysinfo" }

func (*SysInfo) Description() string {
	return "Return a snapshot of system info: kernel/os, uptime, load avg, memory, disk usage."
}

func (*SysInfo) InputSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func (*SysInfo) Run(ctx context.Context, _ map[string]any) (string, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "go_runtime: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	for _, item := range []struct {
		label string
		args  []string
	}{
		{"uname", []string{"uname", "-a"}},
		{"uptime", []string{"uptime"}},
		{"meminfo", []string{"sh", "-c", "free -h 2>/dev/null || vm_stat"}},
		{"disk", []string{"sh", "-c", "df -h / 2>/dev/null"}},
		{"loadavg", []string{"sh", "-c", "cat /proc/loadavg 2>/dev/null"}},
	} {
		out := runShort(ctx, 5*time.Second, item.args[0], item.args[1:]...)
		if out != "" {
			fmt.Fprintf(&sb, "--- %s ---\n%s\n", item.label, out)
		}
	}
	return sb.String(), nil
}

// ProcList returns running processes (top N by RSS).
type ProcList struct{ Limit int }

func NewProcList() *ProcList { return &ProcList{Limit: 30} }

func (*ProcList) Name() string { return "proclist" }

func (*ProcList) Description() string {
	return "List top processes by memory usage. Optional limit (default 30)."
}

func (*ProcList) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"limit": map[string]any{"type": "integer"},
		},
	}
}

func (p *ProcList) Run(ctx context.Context, input map[string]any) (string, error) {
	limit := OptInt(input, "limit", p.Limit)
	if limit <= 0 {
		limit = 30
	}
	cmd := fmt.Sprintf("ps -eo pid,user,pcpu,pmem,rss,etime,comm --sort=-rss 2>/dev/null | head -n %d", limit+1)
	return runShort(ctx, 5*time.Second, "sh", "-c", cmd), nil
}

// LogTail reads the last N lines of a file (like tail -n).
type LogTail struct{ MaxLines int }

func NewLogTail() *LogTail { return &LogTail{MaxLines: 1000} }

func (*LogTail) Name() string { return "logtail" }

func (*LogTail) Description() string {
	return "Read the last N lines of a log file. Use for diagnosing recent failures."
}

func (*LogTail) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":  map[string]any{"type": "string"},
			"lines": map[string]any{"type": "integer"},
		},
		"required": []string{"path"},
	}
}

func (l *LogTail) Run(ctx context.Context, input map[string]any) (string, error) {
	path, err := MustString(input, "path")
	if err != nil {
		return "", err
	}
	n := OptInt(input, "lines", 200)
	if n <= 0 {
		n = 200
	}
	if n > l.MaxLines {
		n = l.MaxLines
	}
	cmd := fmt.Sprintf("tail -n %d %q 2>&1", n, path)
	return runShort(ctx, 10*time.Second, "sh", "-c", cmd), nil
}

func runShort(ctx context.Context, d time.Duration, name string, args ...string) string {
	cctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	out, err := exec.CommandContext(cctx, name, args...).CombinedOutput()
	if err != nil && len(out) == 0 {
		return fmt.Sprintf("error: %v", err)
	}
	return strings.TrimRight(string(out), "\n")
}
