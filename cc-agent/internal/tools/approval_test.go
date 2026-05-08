package tools

import (
	"context"
	"strings"
	"testing"
)

func TestDangerousReason(t *testing.T) {
	cases := []struct {
		cmd   string
		want  string // "" means safe
	}{
		{"ls -la /tmp", ""},
		{"echo hello", ""},
		{"cat /etc/os-release", ""},
		{"grep error /var/log/syslog", ""},
		{"rm -rf /tmp/foo", "recursive"},
		{"rm -rf /", "recursive"},
		{"rm /etc/passwd", "absolute path"},
		{"sudo rm -fr /var", "recursive"},
		{"mkfs.ext4 /dev/sdb1", "mkfs"},
		{"dd if=/dev/zero of=/dev/sda bs=1M", "dd to a block device"},
		{"shutdown -h now", "shutdown"},
		{"systemctl stop nginx", "systemctl stop"},
		{"systemctl disable redis", "systemctl stop"},
		{"iptables -F", "iptables flush"},
		{"kill -9 1234", "SIGKILL"},
		{"chmod -R 777 /", "chmod 777 /"},
		{"git push --force origin main", "git push --force"},
		{":(){ :|:& };:", "fork bomb"},
	}
	for _, c := range cases {
		got := DangerousReason(c.cmd)
		if c.want == "" && got != "" {
			t.Errorf("safe command flagged: %q -> %q", c.cmd, got)
		}
		if c.want != "" && !strings.Contains(strings.ToLower(got), strings.ToLower(c.want)) {
			t.Errorf("dangerous %q: want reason containing %q, got %q", c.cmd, c.want, got)
		}
	}
}

type recordingApprover struct {
	approve bool
	calls   []string
}

func (r *recordingApprover) Approve(_ context.Context, cmd, _ string) (bool, error) {
	r.calls = append(r.calls, cmd)
	return r.approve, nil
}

func TestBashApproval_DenyBlocksExec(t *testing.T) {
	dir := t.TempDir()
	b := NewBash(dir)
	approver := &recordingApprover{approve: false}
	b.Approver = approver

	out, err := b.Run(context.Background(), map[string]any{
		"command": "rm -rf " + dir + "/something",
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "DENIED") {
		t.Errorf("expected DENIED in output: %s", out)
	}
	if len(approver.calls) != 1 {
		t.Errorf("approver should be consulted exactly once, got %d", len(approver.calls))
	}
}

func TestBashApproval_AllowProceeds(t *testing.T) {
	dir := t.TempDir()
	b := NewBash(dir)
	b.Approver = AlwaysApprove{}

	out, err := b.Run(context.Background(), map[string]any{
		"command": "rm -rf " + dir + "/none-such",
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.Contains(out, "DENIED") {
		t.Errorf("AlwaysApprove should not deny: %s", out)
	}
	if !strings.Contains(out, "exit_code:") {
		t.Errorf("expected real execution, got: %s", out)
	}
}

func TestBashApproval_SafeCommandSkipsApprover(t *testing.T) {
	b := NewBash("/tmp")
	approver := &recordingApprover{approve: true}
	b.Approver = approver

	if _, err := b.Run(context.Background(), map[string]any{"command": "echo ok"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(approver.calls) != 0 {
		t.Errorf("approver should not be called for safe command")
	}
}
