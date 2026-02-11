package pty

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

var hostEnvAllowList = []string{
	"PATH", "HOME", "USER", "SHELL", "TERM",
	"LANG", "LC_ALL", "LC_CTYPE",
	"TMPDIR", "XDG_RUNTIME_DIR", "XDG_CONFIG_HOME", "XDG_DATA_HOME",
}

func minimalHostEnv() []string {
	var env []string
	for _, key := range hostEnvAllowList {
		if val, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+val)
		}
	}
	return env
}

type Session struct {
	ID      string
	Cwd     string
	CmdPath string

	mu     sync.RWMutex
	cmd    *exec.Cmd
	ptmx   *os.File
	seq    uint64
	closed chan struct{}
}

func Start(id, cwd, cmdPath string, args []string, env map[string]string, cols, rows uint16) (*Session, error) {
	cmd := exec.Command(cmdPath, args...)
	cmd.Dir = cwd
	cmd.Env = minimalHostEnv()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	if cols > 0 && rows > 0 {
		cmd.Env = append(cmd.Env, "COLUMNS="+itoa(int(cols)), "LINES="+itoa(int(rows)))
	}

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}
	if cols > 0 && rows > 0 {
		_ = pty.Setsize(ptmx, &pty.Winsize{Cols: cols, Rows: rows})
	}

	s := &Session{
		ID:      id,
		Cwd:     cwd,
		CmdPath: cmdPath,
		cmd:     cmd,
		ptmx:    ptmx,
		closed:  make(chan struct{}),
	}
	return s, nil
}

func (s *Session) ReadLoop(onChunk func(seq uint64, chunk []byte), onExit func(code *int, signal, reason string)) {
	s.mu.RLock()
	ptmx := s.ptmx
	s.mu.RUnlock()

	buf := make([]byte, 4096)
	go func() {
		err := s.cmd.Wait()
		var exitCode *int
		sig := ""
		reason := "exited"
		if err != nil {
			var ex *exec.ExitError
			if errors.As(err, &ex) {
				code := ex.ExitCode()
				exitCode = &code
				if ws, ok := ex.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
					sig = ws.Signal().String()
				}
			} else {
				reason = err.Error()
			}
		} else if s.cmd.ProcessState != nil {
			code := s.cmd.ProcessState.ExitCode()
			exitCode = &code
		}
		onExit(exitCode, sig, reason)
		s.Close()
	}()

	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			seq := s.nextSeq()
			onChunk(seq, chunk)
		}
		if err != nil {
			return
		}
	}
}

func (s *Session) Write(p []byte) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.ptmx == nil {
		return errors.New("session closed")
	}
	_, err := s.ptmx.Write(p)
	return err
}

func (s *Session) Resize(cols, rows uint16) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.ptmx == nil {
		return errors.New("session closed")
	}
	return pty.Setsize(s.ptmx, &pty.Winsize{Cols: cols, Rows: rows})
}

func (s *Session) Stop(graceMS, killAfterMS int) {
	if graceMS <= 0 {
		graceMS = 3000
	}
	if killAfterMS <= 0 {
		killAfterMS = 7000
	}
	if killAfterMS < graceMS {
		killAfterMS = graceMS
	}
	s.mu.RLock()
	proc := s.cmd.Process
	s.mu.RUnlock()
	if proc == nil {
		return
	}
	_ = proc.Signal(syscall.SIGTERM)
	time.Sleep(time.Duration(graceMS) * time.Millisecond)
	if s.IsRunning() {
		_ = proc.Kill()
		waitMore := killAfterMS - graceMS
		if waitMore > 0 {
			time.Sleep(time.Duration(waitMore) * time.Millisecond)
		}
	}
}

func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.closed:
		return
	default:
		close(s.closed)
	}
	if s.ptmx != nil {
		_ = s.ptmx.Close()
		s.ptmx = nil
	}
}

func (s *Session) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cmd != nil && s.cmd.ProcessState == nil
}

func (s *Session) nextSeq() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	return s.seq
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return sign + string(b[i:])
}
