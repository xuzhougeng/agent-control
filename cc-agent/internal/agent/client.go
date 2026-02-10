package agent

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	URL            string
	Token          string
	HeartbeatEvery time.Duration
	Manager        *SessionManager
}

func (c *Client) Run(stop <-chan struct{}) error {
	if c.Manager == nil {
		return errors.New("manager required")
	}
	if c.HeartbeatEvery <= 0 {
		c.HeartbeatEvery = 5 * time.Second
	}
	backoff := time.Second
	for {
		select {
		case <-stop:
			return nil
		default:
		}

		connected, err := c.runOnce(stop)
		if err != nil {
			slog.Warn("agent ws disconnected", "err", err)
		}
		if connected {
			backoff = time.Second
		}
		select {
		case <-stop:
			return nil
		case <-time.After(backoff):
		}
		if backoff < 8*time.Second {
			backoff *= 2
		}
	}
}

func (c *Client) runOnce(stop <-chan struct{}) (bool, error) {
	slog.Info("agent connecting", "control_url", c.URL, "server_id", c.Manager.cfg.ServerID)
	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.Token)
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.Dial(c.URL, header)
	if err != nil {
		return false, err
	}
	defer conn.Close()
	slog.Info("agent connected", "control_url", c.URL, "server_id", c.Manager.cfg.ServerID)
	runDone := make(chan struct{})

	send := make(chan Envelope, 256)
	var writeMu sync.Mutex
	sendFunc := func(msg Envelope) error {
		select {
		case send <- msg:
			return nil
		default:
			return errors.New("send queue full")
		}
	}
	c.Manager.SetSendFunc(sendFunc)

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for {
			select {
			case <-runDone:
				return
			case <-stop:
				return
			case msg := <-send:
				writeMu.Lock()
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				err := conn.WriteJSON(msg)
				writeMu.Unlock()
				if err != nil {
					return
				}
			}
		}
	}()

	regData, _ := json.Marshal(c.Manager.RegisterPayload())
	reg := NewEnvelope("register", c.Manager.cfg.ServerID, "")
	reg.Data = regData
	if err := sendFunc(reg); err != nil {
		close(runDone)
		<-writerDone
		return true, err
	}
	slog.Info("agent register sent", "server_id", c.Manager.cfg.ServerID)

	ticker := time.NewTicker(c.HeartbeatEvery)
	defer ticker.Stop()
	go func() {
		for {
			select {
			case <-runDone:
				return
			case <-stop:
				return
			case <-ticker.C:
				hb := NewEnvelope("heartbeat", c.Manager.cfg.ServerID, "")
				_ = sendFunc(hb)
			}
		}
	}()

	for {
		var msg Envelope
		if err := conn.ReadJSON(&msg); err != nil {
			close(runDone)
			<-writerDone
			return true, err
		}
		switch msg.Type {
		case "register_ok":
			slog.Info("agent register_ok received", "server_id", c.Manager.cfg.ServerID)
		case "session_update", "event":
		default:
			if err := c.Manager.Handle(msg); err != nil {
				c.Manager.sendError(msg.SessionID, err.Error())
			}
		}
	}
}

func NormalizeWSURL(base string) (string, error) {
	if strings.HasPrefix(base, "ws://") || strings.HasPrefix(base, "wss://") {
		return base, nil
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	if u.Scheme == "http" {
		u.Scheme = "ws"
	} else if u.Scheme == "https" {
		u.Scheme = "wss"
	}
	return u.String(), nil
}
