package echocli

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

type Message struct {
	MessageID    string          `json:"message_id"`
	Content      string          `json:"content"`
	ContentParts []ContentPart   `json:"content_parts,omitempty"`
	SessionID    string          `json:"session_id,omitempty"`
	Meta         json.RawMessage `json:"meta,omitempty"`
}

type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type ContentPart struct {
	Type   string       `json:"type"`
	Text   string       `json:"text,omitempty"`
	Source *ImageSource `json:"source,omitempty"`
}

type Session struct {
	ID  string
	Cwd string

	mu        sync.Mutex
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	closed    chan struct{}
	onMessage func(Message)
	onExit    func(*int, string)
}

func Start(id, cwd, workerCmd string, workerArgs []string, env map[string]string) (*Session, error) {
	if workerCmd == "" {
		return nil, errors.New("worker_cmd is required")
	}
	c := exec.Command(workerCmd, workerArgs...)
	configureWorkerCmd(c)
	c.Dir = cwd
	c.Stderr = os.Stderr
	c.Env = os.Environ()
	for k, v := range env {
		c.Env = append(c.Env, k+"="+v)
	}

	stdin, err := c.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := c.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, err
	}

	if err := c.Start(); err != nil {
		stdin.Close()
		return nil, err
	}

	s := &Session{
		ID:     id,
		Cwd:    cwd,
		cmd:    c,
		stdin:  stdin,
		closed: make(chan struct{}),
	}

	go s.readLoop(stdout)
	return s, nil
}

func (s *Session) Write(msg Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.closed:
		return errors.New("session closed")
	default:
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = s.stdin.Write(data)
	return err
}

func (s *Session) readLoop(stdout io.ReadCloser) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var msg Message
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		s.mu.Lock()
		cb := s.onMessage
		s.mu.Unlock()
		if cb != nil {
			cb(msg)
		}
	}

	var exitCode *int
	reason := "exited"
	if err := s.cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code := exitErr.ExitCode()
			exitCode = &code
			reason = "exit_code:" + err.Error()
		} else {
			reason = "wait_error:" + err.Error()
		}
	} else {
		code := 0
		exitCode = &code
	}

	s.mu.Lock()
	cb := s.onExit
	s.mu.Unlock()
	if cb != nil {
		cb(exitCode, reason)
	}
	close(s.closed)
}

var _ interface{} = (*Session)(nil)

func (s *Session) SetCallbacks(onMessage func(Message), onExit func(*int, string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onMessage = onMessage
	s.onExit = onExit
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

	s.mu.Lock()
	stdin := s.stdin
	var proc *os.Process
	if s.cmd != nil {
		proc = s.cmd.Process
	}
	s.mu.Unlock()

	if stdin != nil {
		_ = stdin.Close()
	}
	if proc == nil {
		return
	}
	_ = interruptWorkerTree(proc)
	time.Sleep(time.Duration(graceMS) * time.Millisecond)
	if s.IsRunning() {
		_ = killWorkerTree(proc)
		waitMore := killAfterMS - graceMS
		if waitMore > 0 {
			time.Sleep(time.Duration(waitMore) * time.Millisecond)
		}
	}
}

func (s *Session) IsRunning() bool {
	select {
	case <-s.closed:
		return false
	default:
		return true
	}
}
