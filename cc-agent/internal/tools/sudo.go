package tools

import (
	"os/exec"
	"regexp"
	"strings"
)

// AskpassFlag is the hidden argv[1] used when sudo execs this binary as
// SUDO_ASKPASS. It is not a user-facing flag.
const AskpassFlag = "--internal-sudo-askpass"

// sudoWord matches sudo used as a command, not as an argument in echo "sudo".
var sudoWord = regexp.MustCompile(`(?i)(?:^|[;&|\n]|&&|\|\|)\s*sudo\b`)

var (
	sudoStdinFlag = regexp.MustCompile(`(?i)\bsudo\b[^;&|\n]*(\s-S\b|\s--stdin\b)`)
	sudoAskpass   = regexp.MustCompile(`(?i)\bSUDO_ASKPASS=`)
	mysqlPwdFlag  = regexp.MustCompile(`(?i)(\bmysql|\bmariadb|\bpsql|\bredis-cli)\b[^;&|\n]*(\s-p[^\s-]|--password=)`)
	inlineEnvPwd  = regexp.MustCompile(`(?i)\b(MYSQL_PWD|PGPASSWORD|REDISCLI_AUTH)=`)
)

func needsSudo(command string) bool {
	return sudoWord.MatchString(command)
}

// SecretLeakReason is non-empty when the command tries to carry a password
// itself. Those forms are rejected so the secret book stays the only source.
func SecretLeakReason(command string) string {
	if sudoStdinFlag.MatchString(command) {
		return "sudo -S/--stdin (password on stdin)"
	}
	if sudoAskpass.MatchString(command) {
		return "SUDO_ASKPASS in the command"
	}
	if mysqlPwdFlag.MatchString(command) {
		return "database password flag (-pSECRET)"
	}
	if inlineEnvPwd.MatchString(command) {
		return "password environment variable on the command line"
	}
	return ""
}

func sudoAvailable() bool {
	_, err := exec.LookPath("sudo")
	return err == nil
}

func realSudoHasTicket() bool {
	cmd := exec.Command("sudo", "-n", "-v")
	return cmd.Run() == nil
}

func (b *Bash) sudoPresent() bool {
	if b != nil && b.sudoAvailable != nil {
		return b.sudoAvailable()
	}
	return sudoAvailable()
}

func (b *Bash) sudoTicketValid() bool {
	if b != nil && b.sudoHasTicket != nil {
		return b.sudoHasTicket()
	}
	return realSudoHasTicket()
}

func filterAskpassEnv(env []string) []string {
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		if strings.HasPrefix(e, "SUDO_ASKPASS=") {
			continue
		}
		out = append(out, e)
	}
	return out
}
