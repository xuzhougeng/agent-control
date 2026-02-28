package core

import (
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

type fakeAgentConn struct {
	msgs []Envelope
}

func (f *fakeAgentConn) Send(msg Envelope) error {
	f.msgs = append(f.msgs, msg)
	return nil
}

func setupActionTestControlPlane(t *testing.T, prompt string) (*ControlPlane, *fakeAgentConn, string, string) {
	t.Helper()
	auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
	cp, err := NewControlPlane(Config{AuditPath: auditPath})
	if err != nil {
		t.Fatalf("new control plane: %v", err)
	}
	t.Cleanup(func() { _ = cp.Close() })

	conn := &fakeAgentConn{}
	sessionID := "s1"
	instanceID := "inst-1"
	eventID := "e1"
	tenantID := "t1"
	cp.mu.Lock()
	cp.servers["srv"] = &Server{TenantID: tenantID, ServerID: "srv", Status: ServerOnline}
	cp.agentConns["srv"] = conn
	cp.sessions[sessionID] = &Session{
		TenantID:         tenantID,
		SessionID:        sessionID,
		ServerID:         "srv",
		SessionType:      SessionTypePTY,
		ActiveInstanceID: instanceID,
		Status:           SessionRunning,
		AwaitingApproval: true,
		PendingEventID:   eventID,
	}
	cp.instances[instanceID] = &RuntimeInstance{
		InstanceID:       instanceID,
		SessionID:        sessionID,
		TenantID:         tenantID,
		ServerID:         "srv",
		SessionType:      SessionTypePTY,
		Status:           SessionRunning,
		AwaitingApproval: true,
		PendingEventID:   eventID,
	}
	cp.sessionInstances[sessionID] = []string{instanceID}
	cp.sessionEvents[sessionID] = []SessionEvent{
		{
			EventID:    eventID,
			SessionID:  sessionID,
			InstanceID: instanceID,
			ServerID:   "srv",
			TenantID:   tenantID,
			Kind:       "approval_needed",
			PromptText: prompt,
			TsMS:       1,
		},
	}
	cp.mu.Unlock()
	return cp, conn, sessionID, eventID
}

func lastPTYInput(t *testing.T, conn *fakeAgentConn) string {
	t.Helper()
	if len(conn.msgs) == 0 {
		t.Fatal("expected at least one message to agent")
	}
	msg := conn.msgs[len(conn.msgs)-1]
	if msg.Type != "pty_in" {
		t.Fatalf("expected pty_in, got %q", msg.Type)
	}
	raw, err := base64.StdEncoding.DecodeString(msg.DataB64)
	if err != nil {
		t.Fatalf("decode pty input: %v", err)
	}
	return string(raw)
}

func TestHandleClientAction_ApproveMenuUsesEnter(t *testing.T) {
	prompt := "Do you want to create abc?\n1. Yes\n2. Yes, allow all edits during this session (shift+tab)\n3. No\nEsc to cancel · Tab to amend"
	cp, conn, sessionID, eventID := setupActionTestControlPlane(t, prompt)

	if err := cp.HandleClientAction("ui:test", "t1", sessionID, ActionRequest{Kind: "approve", EventID: eventID}); err != nil {
		t.Fatalf("approve action failed: %v", err)
	}
	if got := lastPTYInput(t, conn); got != "\r" {
		t.Fatalf("menu approve should send Enter, got %q", got)
	}
}

func TestHandleClientAction_RejectMenuUsesEscape(t *testing.T) {
	prompt := "Do you want to create abc?\n1. Yes\n2. Yes, allow all edits during this session (shift+tab)\n3. No\nEsc to cancel · Tab to amend"
	cp, conn, sessionID, eventID := setupActionTestControlPlane(t, prompt)

	if err := cp.HandleClientAction("ui:test", "t1", sessionID, ActionRequest{Kind: "reject", EventID: eventID}); err != nil {
		t.Fatalf("reject action failed: %v", err)
	}
	if got := lastPTYInput(t, conn); got != "\u001b" {
		t.Fatalf("menu reject should send Escape, got %q", got)
	}
}

func TestHandleClientAction_PlainPromptUsesYN(t *testing.T) {
	prompt := "Do you want to continue? [y/N]"
	cp1, conn1, sessionID1, eventID1 := setupActionTestControlPlane(t, prompt)
	if err := cp1.HandleClientAction("ui:test", "t1", sessionID1, ActionRequest{Kind: "approve", EventID: eventID1}); err != nil {
		t.Fatalf("approve action failed: %v", err)
	}
	if got := lastPTYInput(t, conn1); got != "y\n" {
		t.Fatalf("plain approve should send y\\n, got %q", got)
	}

	cp2, conn2, sessionID2, eventID2 := setupActionTestControlPlane(t, prompt)
	if err := cp2.HandleClientAction("ui:test", "t1", sessionID2, ActionRequest{Kind: "reject", EventID: eventID2}); err != nil {
		t.Fatalf("reject action failed: %v", err)
	}
	if got := lastPTYInput(t, conn2); got != "n\n" {
		t.Fatalf("plain reject should send n\\n, got %q", got)
	}
}

func TestHandleClientAction_StaleEventIDStillExecutesCurrentPending(t *testing.T) {
	prompt := "Do you want to create abc?\n1. Yes\n2. Yes, allow all edits during this session (shift+tab)\n3. No\nEsc to cancel · Tab to amend"
	cp, conn, sessionID, _ := setupActionTestControlPlane(t, prompt)

	if err := cp.HandleClientAction("ui:test", "t1", sessionID, ActionRequest{Kind: "approve", EventID: "stale-event-id"}); err != nil {
		t.Fatalf("approve with stale event_id should still succeed, got: %v", err)
	}
	if got := lastPTYInput(t, conn); got != "\r" {
		t.Fatalf("menu approve should send Enter for current pending event, got %q", got)
	}
}

func TestDeleteSession_RemovesExitedSessionData(t *testing.T) {
	cp, _, sessionID, _ := setupActionTestControlPlane(t, "Do you want to continue? [y/N]")
	cp.mu.Lock()
	cp.sessions[sessionID].Status = SessionExited
	cp.sessionHubs[sessionID] = newSessionHub(1024)
	cp.mu.Unlock()

	if err := cp.DeleteSession("ui:test", "t1", sessionID); err != nil {
		t.Fatalf("delete session failed: %v", err)
	}

	cp.mu.RLock()
	_, hasSession := cp.sessions[sessionID]
	_, hasEvents := cp.sessionEvents[sessionID]
	_, hasHub := cp.sessionHubs[sessionID]
	cp.mu.RUnlock()
	if hasSession || hasEvents || hasHub {
		t.Fatalf("session artifacts should be removed: session=%v events=%v hub=%v", hasSession, hasEvents, hasHub)
	}
}

func TestCreateApprovalEvent_SetsInstanceID(t *testing.T) {
	cp, _, sessionID, _ := setupActionTestControlPlane(t, "Do you want to continue? [y/N]")
	cp.mu.Lock()
	cp.sessions[sessionID].AwaitingApproval = false
	cp.sessions[sessionID].PendingEventID = ""
	cp.instances["inst-1"].AwaitingApproval = false
	cp.instances["inst-1"].PendingEventID = ""
	cp.sessionEvents[sessionID] = nil
	cp.mu.Unlock()

	cp.createApprovalEvent(sessionID, "inst-1", "srv", "confirm?")

	events := cp.GetSessionEvents("t1", sessionID)
	if len(events) != 1 {
		t.Fatalf("expected 1 approval event, got %d", len(events))
	}
	if events[0].InstanceID != "inst-1" {
		t.Fatalf("expected approval event instance_id inst-1, got %q", events[0].InstanceID)
	}
}

func TestDeleteSession_RejectsActiveSession(t *testing.T) {
	cp, _, sessionID, _ := setupActionTestControlPlane(t, "Do you want to continue? [y/N]")
	cp.mu.Lock()
	cp.sessions[sessionID].Status = SessionRunning
	cp.mu.Unlock()

	if err := cp.DeleteSession("ui:test", "t1", sessionID); err == nil {
		t.Fatal("expected delete active session to fail")
	}
}

func TestStopAndDeleteSession_RunningSendsStopAndRemoves(t *testing.T) {
	cp, conn, sessionID, _ := setupActionTestControlPlane(t, "Do you want to continue? [y/N]")
	cp.mu.Lock()
	cp.sessions[sessionID].Status = SessionRunning
	cp.sessions[sessionID].SessionType = SessionTypePTY
	cp.sessionHubs[sessionID] = newSessionHub(1024)
	cp.mu.Unlock()

	done := make(chan error, 1)
	go func() {
		done <- cp.StopAndDeleteSession("ui:test", "t1", sessionID, 50, 100)
	}()
	time.Sleep(75 * time.Millisecond)
	cp.HandlePTYExit("srv", sessionID, "inst-1", PTYExit{})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("stop and delete failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for stop and delete to finish")
	}
	if len(conn.msgs) == 0 || conn.msgs[len(conn.msgs)-1].Type != "stop_session" {
		t.Fatalf("expected stop_session message before deletion, got %#v", conn.msgs)
	}

	cp.mu.RLock()
	_, hasSession := cp.sessions[sessionID]
	_, hasEvents := cp.sessionEvents[sessionID]
	_, hasHub := cp.sessionHubs[sessionID]
	cp.mu.RUnlock()
	if hasSession || hasEvents || hasHub {
		t.Fatalf("session artifacts should be removed: session=%v events=%v hub=%v", hasSession, hasEvents, hasHub)
	}
}

func TestStopAndDeleteSession_ExitedDeletesDirectly(t *testing.T) {
	cp, conn, sessionID, _ := setupActionTestControlPlane(t, "Do you want to continue? [y/N]")
	cp.mu.Lock()
	cp.sessions[sessionID].Status = SessionExited
	cp.sessionHubs[sessionID] = newSessionHub(1024)
	cp.mu.Unlock()

	if err := cp.StopAndDeleteSession("ui:test", "t1", sessionID, 0, 0); err != nil {
		t.Fatalf("stop and delete failed: %v", err)
	}
	if len(conn.msgs) != 0 {
		t.Fatalf("exited session should not send stop message, got %#v", conn.msgs)
	}
}

func TestStopAndDeleteSession_RunningChatWaitsForExit(t *testing.T) {
	cp, conn, sessionID, _ := setupActionTestControlPlane(t, "Do you want to continue? [y/N]")
	cp.mu.Lock()
	cp.sessions[sessionID].Status = SessionRunning
	cp.sessions[sessionID].SessionType = SessionTypeChat
	cp.sessionHubs[sessionID] = newSessionHub(1024)
	cp.mu.Unlock()

	done := make(chan error, 1)
	go func() {
		done <- cp.StopAndDeleteSession("ui:test", "t1", sessionID, 50, 100)
	}()

	select {
	case err := <-done:
		t.Fatalf("stop and delete returned before chat exit: %v", err)
	case <-time.After(75 * time.Millisecond):
	}

	cp.HandleChatExit("srv", sessionID, "inst-1", nil)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("stop and delete failed after chat exit: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for stop and delete to finish after chat exit")
	}

	if len(conn.msgs) == 0 || conn.msgs[len(conn.msgs)-1].Type != "stop_session" {
		t.Fatalf("expected stop_session message before deletion, got %#v", conn.msgs)
	}

	cp.mu.RLock()
	_, hasSession := cp.sessions[sessionID]
	cp.mu.RUnlock()
	if hasSession {
		t.Fatal("session should be removed after chat exit")
	}
}

func TestStopAndDeleteSession_RunningPTYWaitsForExit(t *testing.T) {
	cp, conn, sessionID, _ := setupActionTestControlPlane(t, "Do you want to continue? [y/N]")
	cp.mu.Lock()
	cp.sessions[sessionID].Status = SessionRunning
	cp.sessions[sessionID].SessionType = SessionTypePTY
	cp.sessionHubs[sessionID] = newSessionHub(1024)
	cp.mu.Unlock()

	done := make(chan error, 1)
	go func() {
		done <- cp.StopAndDeleteSession("ui:test", "t1", sessionID, 50, 100)
	}()

	select {
	case err := <-done:
		t.Fatalf("stop and delete returned before pty exit: %v", err)
	case <-time.After(75 * time.Millisecond):
	}

	cp.HandlePTYExit("srv", sessionID, "inst-1", PTYExit{})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("stop and delete failed after pty exit: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for stop and delete to finish after pty exit")
	}

	if len(conn.msgs) == 0 || conn.msgs[len(conn.msgs)-1].Type != "stop_session" {
		t.Fatalf("expected stop_session message before deletion, got %#v", conn.msgs)
	}

	cp.mu.RLock()
	_, hasSession := cp.sessions[sessionID]
	cp.mu.RUnlock()
	if hasSession {
		t.Fatal("session should be removed after pty exit")
	}
}

func TestSwitchSessionMode_PTYToChatReusesLogicalSessionAndNewInstance(t *testing.T) {
	cp, conn, sessionID, _ := setupActionTestControlPlane(t, "Do you want to continue? [y/N]")
	cp.mu.Lock()
	cp.sessions[sessionID].Status = SessionRunning
	cp.sessions[sessionID].SessionType = SessionTypePTY
	cp.mu.Unlock()
	conn.msgs = nil

	done := make(chan struct {
		sess *Session
		err  error
	}, 1)
	go func() {
		sess, err := cp.SwitchSessionMode("ui:test", "t1", sessionID, SwitchSessionRequest{
			SessionType: SessionTypeChat,
			Env:         map[string]string{"FOO": "bar"},
		})
		done <- struct {
			sess *Session
			err  error
		}{sess: sess, err: err}
	}()

	select {
	case res := <-done:
		t.Fatalf("switch returned before pty exit: %+v", res)
	case <-time.After(75 * time.Millisecond):
	}

	if len(conn.msgs) != 1 || conn.msgs[0].Type != "stop_session" {
		t.Fatalf("expected only stop_session before exit, got %#v", conn.msgs)
	}
	if conn.msgs[0].SessionID != sessionID || conn.msgs[0].InstanceID != "inst-1" {
		t.Fatalf("unexpected stop target: %#v", conn.msgs[0])
	}

	cp.HandlePTYExit("srv", sessionID, "inst-1", PTYExit{})

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("switch failed: %v", res.err)
		}
		if res.sess == nil {
			t.Fatal("expected updated session")
		}
		if res.sess.SessionID != sessionID {
			t.Fatalf("expected logical session id %s, got %s", sessionID, res.sess.SessionID)
		}
		if res.sess.SessionType != SessionTypeChat {
			t.Fatalf("expected chat session type, got %s", res.sess.SessionType)
		}
		if res.sess.ActiveInstanceID == "" || res.sess.ActiveInstanceID == "inst-1" {
			t.Fatalf("expected new active instance id, got %q", res.sess.ActiveInstanceID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for switch to finish")
	}

	if len(conn.msgs) != 2 {
		t.Fatalf("expected stop + start_chat, got %#v", conn.msgs)
	}
	startMsg := conn.msgs[1]
	if startMsg.Type != "start_chat" {
		t.Fatalf("expected start_chat, got %#v", startMsg)
	}
	if startMsg.SessionID != sessionID {
		t.Fatalf("expected reused logical session id, got %s", startMsg.SessionID)
	}
	if startMsg.InstanceID == "" || startMsg.InstanceID == "inst-1" {
		t.Fatalf("expected new runtime instance id, got %q", startMsg.InstanceID)
	}
	instances := cp.GetSessionInstances("t1", sessionID)
	if len(instances) != 2 {
		t.Fatalf("expected 2 runtime instances, got %d", len(instances))
	}
	if instances[0].InstanceID != startMsg.InstanceID {
		t.Fatalf("expected newest instance to be active start instance, got %q", instances[0].InstanceID)
	}
	if instances[1].InstanceID != "inst-1" {
		t.Fatalf("expected prior instance history entry, got %q", instances[1].InstanceID)
	}
}

func TestSwitchSessionMode_ChatToPTYReusesLogicalSessionAndNewInstance(t *testing.T) {
	cp, conn, sessionID, _ := setupActionTestControlPlane(t, "Do you want to continue? [y/N]")
	cp.mu.Lock()
	cp.sessions[sessionID].Status = SessionRunning
	cp.sessions[sessionID].SessionType = SessionTypeChat
	cp.instances["inst-1"].SessionType = SessionTypeChat
	cp.mu.Unlock()
	conn.msgs = nil

	done := make(chan struct {
		sess *Session
		err  error
	}, 1)
	go func() {
		sess, err := cp.SwitchSessionMode("ui:test", "t1", sessionID, SwitchSessionRequest{
			SessionType: SessionTypePTY,
			Env:         map[string]string{"BAR": "baz"},
			Cols:        120,
			Rows:        30,
		})
		done <- struct {
			sess *Session
			err  error
		}{sess: sess, err: err}
	}()

	select {
	case res := <-done:
		t.Fatalf("switch returned before chat exit: %+v", res)
	case <-time.After(75 * time.Millisecond):
	}

	if len(conn.msgs) != 1 || conn.msgs[0].Type != "stop_session" {
		t.Fatalf("expected only stop_session before exit, got %#v", conn.msgs)
	}

	cp.HandleChatExit("srv", sessionID, "inst-1", nil)

	var res struct {
		sess *Session
		err  error
	}
	select {
	case res = <-done:
		if res.err != nil {
			t.Fatalf("switch failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for switch to finish")
	}

	if res.sess == nil {
		t.Fatal("expected updated session")
	}
	if res.sess.SessionID != sessionID {
		t.Fatalf("expected logical session id %s, got %s", sessionID, res.sess.SessionID)
	}
	if res.sess.SessionType != SessionTypePTY {
		t.Fatalf("expected pty session type, got %s", res.sess.SessionType)
	}
	if res.sess.ActiveInstanceID == "" || res.sess.ActiveInstanceID == "inst-1" {
		t.Fatalf("expected new active instance id, got %q", res.sess.ActiveInstanceID)
	}

	if len(conn.msgs) != 2 {
		t.Fatalf("expected stop + start_session, got %#v", conn.msgs)
	}
	startMsg := conn.msgs[1]
	if startMsg.Type != "start_session" {
		t.Fatalf("expected start_session, got %#v", startMsg)
	}
	if startMsg.SessionID != sessionID {
		t.Fatalf("expected reused logical session id, got %s", startMsg.SessionID)
	}
	if startMsg.InstanceID == "" || startMsg.InstanceID == "inst-1" {
		t.Fatalf("expected new runtime instance id, got %q", startMsg.InstanceID)
	}

	var payload struct {
		Cmd []string `json:"cmd"`
	}
	if err := json.Unmarshal(startMsg.Data, &payload); err != nil {
		t.Fatalf("unmarshal start payload: %v", err)
	}
	if len(payload.Cmd) < 2 || payload.Cmd[1] != "--session-id" {
		t.Fatalf("expected pty start command to include --session-id, got %#v", payload.Cmd)
	}
	if len(payload.Cmd) < 3 || payload.Cmd[2] != sessionID {
		t.Fatalf("expected claude logical session id reuse, got %#v", payload.Cmd)
	}
}

func TestSwitchSessionMode_ChatWithHistoryToPTYUsesResume(t *testing.T) {
	cp, conn, sessionID, _ := setupActionTestControlPlane(t, "Do you want to continue? [y/N]")
	cp.mu.Lock()
	cp.sessions[sessionID].Status = SessionRunning
	cp.sessions[sessionID].SessionType = SessionTypeChat
	cp.instances["inst-1"].SessionType = SessionTypeChat
	cp.chatHistory[sessionID] = []ChatMessage{{
		MessageID:  "m1",
		SessionID:  sessionID,
		InstanceID: "inst-1",
		Role:       "user",
		Content:    "hi",
		TsMS:       time.Now().UnixMilli(),
	}}
	cp.mu.Unlock()
	conn.msgs = nil

	done := make(chan struct {
		sess *Session
		err  error
	}, 1)
	go func() {
		sess, err := cp.SwitchSessionMode("ui:test", "t1", sessionID, SwitchSessionRequest{
			SessionType: SessionTypePTY,
			Env:         map[string]string{"BAR": "baz"},
			Cols:        120,
			Rows:        30,
		})
		done <- struct {
			sess *Session
			err  error
		}{sess: sess, err: err}
	}()

	select {
	case res := <-done:
		t.Fatalf("switch returned before chat exit: %+v", res)
	case <-time.After(75 * time.Millisecond):
	}

	cp.HandleChatExit("srv", sessionID, "inst-1", nil)

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("switch failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for switch to finish")
	}

	if len(conn.msgs) != 2 {
		t.Fatalf("expected stop + start_session, got %#v", conn.msgs)
	}
	startMsg := conn.msgs[1]
	var payload struct {
		Cmd []string `json:"cmd"`
	}
	if err := json.Unmarshal(startMsg.Data, &payload); err != nil {
		t.Fatalf("unmarshal start payload: %v", err)
	}
	if len(payload.Cmd) < 2 || payload.Cmd[1] != "--resume" {
		t.Fatalf("expected pty start command to include --resume, got %#v", payload.Cmd)
	}
	if len(payload.Cmd) < 3 || payload.Cmd[2] != sessionID {
		t.Fatalf("expected claude logical session id for resume, got %#v", payload.Cmd)
	}
}

func TestSwitchSessionMode_PTYSansConversationHistoryKeepsSessionID(t *testing.T) {
	cp, conn, sessionID, _ := setupActionTestControlPlane(t, "Do you want to continue? [y/N]")
	cp.mu.Lock()
	cp.sessions[sessionID].Status = SessionRunning
	cp.sessions[sessionID].SessionType = SessionTypeChat
	cp.instances["inst-1"].SessionType = SessionTypeChat
	ptyInst := &RuntimeInstance{
		InstanceID:  "inst-pty",
		SessionID:   sessionID,
		TenantID:    "t1",
		ServerID:    "srv",
		SessionType: SessionTypePTY,
		Status:      SessionExited,
	}
	cp.instances[ptyInst.InstanceID] = ptyInst
	cp.sessionInstances[sessionID] = append(cp.sessionInstances[sessionID], ptyInst.InstanceID)
	cp.mu.Unlock()
	conn.msgs = nil

	done := make(chan struct {
		sess *Session
		err  error
	}, 1)
	go func() {
		sess, err := cp.SwitchSessionMode("ui:test", "t1", sessionID, SwitchSessionRequest{
			SessionType: SessionTypePTY,
			Env:         map[string]string{"BAR": "baz"},
			Cols:        120,
			Rows:        30,
		})
		done <- struct {
			sess *Session
			err  error
		}{sess: sess, err: err}
	}()

	select {
	case res := <-done:
		t.Fatalf("switch returned before chat exit: %+v", res)
	case <-time.After(75 * time.Millisecond):
	}

	cp.HandleChatExit("srv", sessionID, "inst-1", nil)

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("switch failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for switch to finish")
	}

	if len(conn.msgs) != 2 {
		t.Fatalf("expected stop + start_session, got %#v", conn.msgs)
	}
	startMsg := conn.msgs[1]
	var payload struct {
		Cmd []string `json:"cmd"`
	}
	if err := json.Unmarshal(startMsg.Data, &payload); err != nil {
		t.Fatalf("unmarshal start payload: %v", err)
	}
	if len(payload.Cmd) < 2 || payload.Cmd[1] != "--session-id" {
		t.Fatalf("expected pty start command to include --session-id, got %#v", payload.Cmd)
	}
	if len(payload.Cmd) < 3 || payload.Cmd[2] != sessionID {
		t.Fatalf("expected claude logical session id reuse, got %#v", payload.Cmd)
	}
}

func TestSwitchSessionMode_ReusesPriorModeInstanceInsteadOfAppendingForever(t *testing.T) {
	cp, conn, sessionID, _ := setupActionTestControlPlane(t, "Do you want to continue? [y/N]")
	cp.mu.Lock()
	cp.sessions[sessionID].Status = SessionRunning
	cp.sessions[sessionID].SessionType = SessionTypeChat
	cp.instances["inst-1"].SessionType = SessionTypeChat
	cp.mu.Unlock()
	conn.msgs = nil

	switchOnce := func(target SessionType, exitFn func()) string {
		done := make(chan struct {
			sess *Session
			err  error
		}, 1)
		go func() {
			sess, err := cp.SwitchSessionMode("ui:test", "t1", sessionID, SwitchSessionRequest{
				SessionType: target,
				Env:         map[string]string{"MODE": string(target)},
				Cols:        120,
				Rows:        30,
			})
			done <- struct {
				sess *Session
				err  error
			}{sess: sess, err: err}
		}()

		select {
		case res := <-done:
			t.Fatalf("switch returned before exit: %+v", res)
		case <-time.After(75 * time.Millisecond):
		}

		exitFn()

		select {
		case res := <-done:
			if res.err != nil {
				t.Fatalf("switch failed: %v", res.err)
			}
			if res.sess == nil {
				t.Fatal("expected updated session")
			}
			return res.sess.ActiveInstanceID
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for switch to finish")
			return ""
		}
	}

	ptyInstanceID := switchOnce(SessionTypePTY, func() {
		cp.HandleChatExit("srv", sessionID, "inst-1", nil)
	})
	if ptyInstanceID == "" || ptyInstanceID == "inst-1" {
		t.Fatalf("expected distinct pty instance id, got %q", ptyInstanceID)
	}

	chatInstanceID := switchOnce(SessionTypeChat, func() {
		cp.HandlePTYExit("srv", sessionID, ptyInstanceID, PTYExit{})
	})
	if chatInstanceID != "inst-1" {
		t.Fatalf("expected chat switch-back to reuse original chat instance, got %q", chatInstanceID)
	}

	instances := cp.GetSessionInstances("t1", sessionID)
	if len(instances) != 2 {
		t.Fatalf("expected instance slots to stay capped at 2, got %d", len(instances))
	}
	seen := map[string]bool{}
	for _, inst := range instances {
		seen[inst.InstanceID] = true
	}
	if !seen["inst-1"] || !seen[ptyInstanceID] {
		t.Fatalf("expected chat and pty instance slots to remain, got %#v", instances)
	}
}
