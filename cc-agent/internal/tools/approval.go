package tools

import (
	"context"
	"regexp"
)

// Approver decides whether a destructive shell command may run. The Bash tool
// calls Approve before executing any command flagged by DangerousReason; an
// approval miss returns the operator-supplied reason as the tool result so
// the model sees the rejection.
type Approver interface {
	Approve(ctx context.Context, command, reason string) (bool, error)
}

// AlwaysApprove is the silent rubber stamp. Useful in tests and when the user
// has explicitly opted out via -no-approval.
type AlwaysApprove struct{}

func (AlwaysApprove) Approve(_ context.Context, _, _ string) (bool, error) { return true, nil }

// AlwaysDeny rejects everything. The default for HTTP / programmatic mode
// where we cannot prompt a human.
type AlwaysDeny struct{}

func (AlwaysDeny) Approve(_ context.Context, _, _ string) (bool, error) { return false, nil }

// dangerousMatchers covers a small set of high-impact patterns. The intent
// is: catch obvious foot-guns; defense in depth lives elsewhere (cwd
// containment, filesystem permissions, role-based controls).
//
// New patterns should each be paired with a clear `reason` so the operator
// understands why the prompt fired.
var dangerousMatchers = []struct {
	re     *regexp.Regexp
	reason string
}{
	{regexp.MustCompile(`(?i)\brm\s+(-[rRfFiv]*[rR][rRfFiv]*|--recursive)`), "recursive rm"},
	{regexp.MustCompile(`(?i)\brm\s+(-[rRfFiv]*[fF][rRfFiv]*|--force)\s+/`), "rm -f /"},
	{regexp.MustCompile(`(?i)\brm\s+/[^/\s]`), "rm of an absolute path"},
	{regexp.MustCompile(`(?i)\bmkfs\b`), "mkfs (filesystem create)"},
	{regexp.MustCompile(`(?i)\bdd\s+.*of=/dev/(sd|nvme|hd|vd|mmcblk)`), "dd to a block device"},
	{regexp.MustCompile(`(?i)>\s*/dev/(sd|nvme|hd|vd|mmcblk)`), "redirect to a block device"},
	{regexp.MustCompile(`(?i)\b(shutdown|reboot|halt|poweroff)\b`), "shutdown / reboot"},
	{regexp.MustCompile(`(?i)\bsystemctl\s+(stop|disable|mask|kill|reboot|poweroff|halt)\b`), "systemctl stop/disable/mask"},
	{regexp.MustCompile(`(?i)\bservice\s+\S+\s+(stop|restart|reload)\b`), "service stop/restart"},
	{regexp.MustCompile(`(?i)\biptables\s+(-F|--flush)\b`), "iptables flush"},
	{regexp.MustCompile(`(?i)\bufw\s+disable\b`), "ufw disable"},
	{regexp.MustCompile(`(?i)\bkill(all)?\s+(-9|-KILL|-SIGKILL)\b`), "SIGKILL"},
	{regexp.MustCompile(`(?i)\b(userdel|groupdel)\b`), "user / group delete"},
	{regexp.MustCompile(`(?i)\bchmod\s+(-R\s+)?[0-7]?777\s+/`), "chmod 777 /"},
	{regexp.MustCompile(`(?i)\bchown\s+.*\s+/$`), "chown /"},
	{regexp.MustCompile(`(?i):\(\)\s*\{[^}]*:\|`), "fork bomb"},
	{regexp.MustCompile(`(?i)\bgit\s+push\s+(-f|--force)\b`), "git push --force"},
	{regexp.MustCompile(`(?i)\bgit\s+(reset\s+--hard|clean\s+-fd)`), "git reset --hard / clean -fd"},
}

// DangerousReason returns a non-empty human-readable reason if the command
// matches a destructive pattern; empty string otherwise.
func DangerousReason(command string) string {
	for _, m := range dangerousMatchers {
		if m.re.MatchString(command) {
			return m.reason
		}
	}
	return ""
}
