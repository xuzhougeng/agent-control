package tools

import "testing"

func TestNeedsSudo(t *testing.T) {
	yes := []string{
		"sudo apt update",
		"sudo -u root id",
		"true && sudo reboot",
		"echo hi; sudo ls",
	}
	no := []string{
		"echo sudo",
		"ls /usr/bin/sudo",
		"grep sudo /var/log/auth.log",
	}
	for _, c := range yes {
		if !needsSudo(c) {
			t.Errorf("expected sudo: %q", c)
		}
	}
	for _, c := range no {
		if needsSudo(c) {
			t.Errorf("false sudo: %q", c)
		}
	}
}

func TestSecretLeakReason(t *testing.T) {
	cases := []struct {
		cmd  string
		want bool
	}{
		{"sudo apt update", false},
		{"sudo -S apt update", true},
		{"sudo --stdin cat /etc/shadow", true},
		{"SUDO_ASKPASS=/tmp/x sudo ls", true},
		{"mysql -uroot -psecret testdb", true},
		{"mysql --password=secret testdb", true},
		{"MYSQL_PWD=secret mysql -e 'select 1'", true},
		{"PGPASSWORD=x psql -c 'select 1'", true},
		{"mysql -h db -u app -p testdb", false}, // -p with space prompts; no secret inline
	}
	for _, c := range cases {
		got := SecretLeakReason(c.cmd)
		if c.want && got == "" {
			t.Errorf("expected leak for %q", c.cmd)
		}
		if !c.want && got != "" {
			t.Errorf("unexpected leak %q for %q", got, c.cmd)
		}
	}
}
