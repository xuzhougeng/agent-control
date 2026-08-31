import { PROTOCOL_VERSION } from "../shared/ws.js";

/**
 * @param {object} ctx
 * @param {object} ctx.state
 * @param {object} ctx.e2eState
 * @param {object} ctx.terminal
 * @param {object} ctx.chat
 * @param {object} ctx.session
 */
export function createWSHandler(ctx) {
  let wsClient = null;

  function setWSClient(client) {
    wsClient = client;
  }

  function decodeB64Text(b64) {
    try {
      const bin = atob(String(b64 || ""));
      const arr = new Uint8Array(bin.length);
      for (let i = 0; i < bin.length; i += 1) arr[i] = bin.charCodeAt(i);
      return new TextDecoder().decode(arr);
    } catch {
      return "";
    }
  }

  function isProgressMessage(chatMsg) {
    const meta = chatMsg && typeof chatMsg === "object" ? chatMsg.meta : null;
    if (!meta || typeof meta !== "object") return false;
    return meta.source === "claude-stream-json" && meta.progress === true;
  }

  function handleWS(msg) {
    if (msg.type === "hello") {
      const minVersion = msg.data?.protocol_version_min ?? 0;
      if (PROTOCOL_VERSION < minVersion) {
        alert("Your browser client is outdated. Please refresh the page.");
        wsClient?.getSocket()?.close();
        return;
      }
      return;
    }
    if (msg.type === "term_out" && msg.session_id === ctx.state.selectedSessionID && msg.data_b64) {
      const chunk = decodeB64Text(msg.data_b64);
      ctx.e2eState.lastTermOutAtMs = Date.now();
      ctx.e2eState.lastTermOutSessionID = String(msg.session_id || "");
      ctx.e2eState.termOutCount += 1;
      ctx.e2eState.termOutRecentText = `${ctx.e2eState.termOutRecentText}${chunk}`.slice(-2048);
      ctx.terminal.writeOutput(msg, ctx.state.pendingFirstOutputSessionID, () => {
        ctx.state.pendingFirstOutputSessionID = "";
        ctx.terminal.markLoaded(msg.session_id, msg.instance_id || "");
      });
      return;
    }

    if (msg.type === "chat_msg" && msg.data) {
      const cm = msg.data;
      if (cm.session_id === ctx.state.selectedSessionID) {
        ctx.state.chatMessages.push(cm);
        if (cm.role === "assistant" && !isProgressMessage(cm)) {
          ctx.chat.completePendingTurn();
        }
        ctx.chat.renderChatMessages();
      }
      return;
    }

    if (msg.type === "event" && msg.data) {
      const ev = msg.data;
      if (ev.kind === "notification") {
        ctx.session.pushNotification(ev);
        return;
      }
      if (ev.kind === "approval_needed" || ev.kind === "credential_needed") {
        ctx.state.approvals.set(ev.event_id, ev);
        ctx.session.renderApprovals();
      }
      return;
    }

    if (msg.type === "session_update" && msg.data) {
      const data = msg.data;
      if (data.session_id === ctx.state.selectedSessionID && (data.status === "error" || data.status === "exited")) {
        ctx.chat.failPendingTurns(data.exit_reason || `session ${data.status}`);
      }
      if (!data.awaiting_approval) {
        for (const ev of ctx.state.approvals.values()) {
          if (ev.session_id === data.session_id && !ev.resolved) {
            ev.resolved = true;
            ctx.state.approvals.set(ev.event_id, ev);
          }
        }
        ctx.session.renderApprovals();
      }
      ctx.session.fetchSessions();
      if (data.session_id === ctx.state.selectedSessionID) {
        ctx.session.fetchSessionInstances(ctx.state.selectedSessionID);
      }
    }
  }

  return { handleWS, decodeB64Text, setWSClient };
}
