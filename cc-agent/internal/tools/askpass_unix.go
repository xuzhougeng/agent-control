//go:build unix

package tools

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RunAskpass is the hidden helper sudo execs via SUDO_ASKPASS. It only
// speaks to the in-process unix socket; the password never hits disk.
// Exit codes are silent on stdout so a failure cannot be mistaken for a password.
func RunAskpass(sock string) int {
	if sock == "" || !parentLooksLikeSudo() {
		return 1
	}
	conn, err := net.DialTimeout("unix", sock, 2*time.Second)
	if err != nil {
		return 1
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.Copy(os.Stdout, conn); err != nil {
		return 1
	}
	return 0
}

func parentLooksLikeSudo() bool {
	ppid := os.Getppid()
	if ppid <= 1 {
		return false
	}
	if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", ppid)); err == nil {
		return strings.TrimSpace(string(b)) == "sudo"
	}
	cmd := exec.Command("ps", "-o", "comm=", "-p", fmt.Sprintf("%d", ppid))
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "sudo"
}

type askpassSession struct {
	ln     net.Listener
	script string
	sock   string
}

func startSudoAskpass(secret []byte) (apply func(*exec.Cmd), cleanup func(), err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, err
	}
	dir := filepath.Join(home, ".cc-agent")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, err
	}
	sock := filepath.Join(dir, "askpass.sock")
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return nil, nil, err
	}
	if err := os.Chmod(sock, 0o600); err != nil {
		_ = ln.Close()
		_ = os.Remove(sock)
		return nil, nil, err
	}

	exe, err := os.Executable()
	if err != nil {
		_ = ln.Close()
		_ = os.Remove(sock)
		return nil, nil, err
	}
	script := filepath.Join(dir, "askpass")
	body := fmt.Sprintf("#!/bin/sh\nexec %s %s %s\n", shellQuote(exe), AskpassFlag, shellQuote(sock))
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		_ = ln.Close()
		_ = os.Remove(sock)
		return nil, nil, err
	}

	held := append([]byte(nil), secret...)
	sess := &askpassSession{ln: ln, script: script, sock: sock}
	go sess.serveOnce(held)

	apply = func(cmd *exec.Cmd) {
		env := cmd.Env
		if env == nil {
			env = os.Environ()
		}
		cmd.Env = append(filterAskpassEnv(env), "SUDO_ASKPASS="+script)
	}
	cleanup = func() {
		_ = ln.Close()
		_ = os.Remove(sock)
		// Keep the reusable 0700 askpass script; it contains no secret.
		for i := range held {
			held[i] = 0
		}
	}
	return apply, cleanup, nil
}

func (s *askpassSession) serveOnce(secret []byte) {
	defer func() {
		for i := range secret {
			secret[i] = 0
		}
	}()
	conn, err := s.ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, _ = conn.Write(secret)
	if len(secret) == 0 || secret[len(secret)-1] != '\n' {
		_, _ = conn.Write([]byte("\n"))
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
