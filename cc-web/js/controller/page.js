import { createUIApi } from "../shared/http.js";
import { createSidebarController } from "../shared/sidebar.js";
import { createWSClient, setWSBadge } from "../shared/ws.js";
import { createTerminalController } from "./terminal.js";
import {
  WINDOWS_PTY_UNSUPPORTED_ERROR,
  escapeHtml,
  parseEnv,
} from "../shared/utils.js";
import { renderServerList } from "../shared/server-list.js";
import { renderSessionList } from "../shared/session-list.js";

export function initControllerPage() {
  const query = new URLSearchParams(window.location.search);
  const state = {
    token: localStorage.getItem("ui_token") || "admin-dev-token",
    ws: null,
    selectedServerID: query.get("server_id") || "",
    requestedSessionID: query.get("session_id") || "",
    selectedSessionID: "",
    pendingFirstOutputSessionID: "",
    approvals: new Map(),
    sessions: [],
    servers: [],
  };

  const api = createUIApi(() => state.token);

  const tokenInput = document.getElementById("tokenInput");
  const saveTokenBtn = document.getElementById("saveTokenBtn");
  const serversList = document.getElementById("serversList");
  const sessionsList = document.getElementById("sessionsList");
  const approvalList = document.getElementById("approvalList");
  const approvalCount = document.getElementById("approvalCount");
  const approvalDetails = document.getElementById("approvalDetails");
  const cwdInput = document.getElementById("cwdInput");
  const sessionIDInput = document.getElementById("sessionIdInput");
  const envInput = document.getElementById("envInput");

  function sendWS(msg) {
    return wsClient.send(msg);
  }

  const terminal = createTerminalController({
    getSelectedSessionID: () => state.selectedSessionID,
    sendWS,
    onTermData: () => {},
  });

  const sidebar = createSidebarController({
    isControllerPage: true,
    isChatPage: false,
    onLayoutChange: () => terminal.syncLayout(),
    approvalDetails,
  });

  const wsClient = createWSClient({
    getToken: () => state.token,
    shouldConnect: () => true,
    setStatus: setWSBadge,
    onOpen: ({ send }) => {
      state.ws = wsClient.getSocket();
      if (state.selectedSessionID) {
        send({ type: "attach", data: { session_id: state.selectedSessionID, since_seq: 0 } });
      }
      terminal.sendResize();
    },
    onMessage: (msg) => handleWS(msg),
    onError: (e) => console.error("[ws] error", e),
  });

  function getServerByID(serverID) {
    if (!serverID) return null;
    return state.servers.find((s) => s.server_id === serverID) || null;
  }

  function getSelectedServer() {
    return getServerByID(state.selectedServerID);
  }

  function isWindowsServer(server) {
    return String((server && server.os) || "").toLowerCase() === "windows";
  }

  function maybeRedirectWindowsServerToChat(server) {
    if (!isWindowsServer(server)) return false;
    const serverID = (server && server.server_id) || state.selectedServerID || "";
    alert("Selected server is Windows. PTY is not supported yet; switching to Chat mode.");
    const query = serverID ? `?server_id=${encodeURIComponent(serverID)}` : "";
    window.location.href = `/chat${query}`;
    return true;
  }

  function formatSessionCreateError(rawText) {
    const msg = String(rawText || "").trim();
    if (msg.includes(WINDOWS_PTY_UNSUPPORTED_ERROR)) {
      return "PTY is not supported on Windows yet; please use Chat mode at /chat.";
    }
    if (msg.includes("invalid session_id")) {
      return "session_id must be a valid UUID.";
    }
    if (msg.includes("session_id already exists")) {
      return "session_id already exists.";
    }
    return msg || "request failed";
  }

  function renderServers() {
    renderServerList(serversList, state.servers, state.selectedServerID, async (server) => {
      state.selectedServerID = server.server_id;
      renderServers();
      if (maybeRedirectWindowsServerToChat(server)) return;
      await fetchSessions();
      sidebar.closeSidebarOnMobile();
    });
  }

  function renderSessions() {
    renderSessionList({
      listEl: sessionsList,
      sessions: state.sessions,
      wantType: "pty",
      selectedSessionID: state.selectedSessionID,
      renderItem: (s, isSelected) => {
      const li = document.createElement("li");
      li.classList.add("session-item");
      if (isSelected) li.classList.add("selected");
      const statusBadge = s.status === "running" ? "badge badge-running" : "badge";
      const canDelete = s.status !== "running";
      const approvalClass = s.awaiting_approval ? "badge-pending" : "badge-muted";
      li.innerHTML = `
        <div class="session-main">
          <strong class="session-id">${escapeHtml(s.session_id.slice(0, 8))}</strong>
          <div class="session-badges">
            <span class="${statusBadge}">${escapeHtml(s.status)}</span>
            <span class="badge ${approvalClass}">${s.awaiting_approval ? "approval" : "normal"}</span>
          </div>
        </div>
        <div class="session-sub">${escapeHtml(s.cwd || "-")}</div>
        ${s.exit_reason
          ? `<div class="session-detail"><span>reason ${escapeHtml(s.exit_reason)}</span></div>`
          : ""}
        <div class="session-actions">
          <button type="button" data-action="chat" class="btn-secondary">Open Chat</button>
          <button type="button" data-action="delete" class="btn-danger" ${canDelete ? "" : "disabled"}>Delete</button>
        </div>
      `;
      li.querySelector('[data-action="chat"]').addEventListener("click", async (e) => {
        e.stopPropagation();
        await switchSessionToChat(s);
      });
      li.querySelector('[data-action="delete"]').addEventListener("click", async (e) => {
        e.stopPropagation();
        await deleteSession(s);
      });
      li.addEventListener("click", () => attachSession(s.session_id));
      return li;
      },
    });
  }

  function renderApprovals() {
    approvalList.innerHTML = "";
    const values = Array.from(state.approvals.values()).sort((a, b) => b.ts_ms - a.ts_ms);
    let pendingCount = 0;
    for (const ev of values) {
      if (ev.resolved) continue;
      pendingCount += 1;
      const li = document.createElement("li");
      li.classList.add("approval-item");
      li.tabIndex = 0;
      li.setAttribute("role", "button");
      li.setAttribute("aria-label", `Open approval for session ${ev.session_id.slice(0, 8)} on ${ev.server_id}`);
      li.innerHTML = `<div><strong>${escapeHtml(ev.session_id.slice(0, 8))}</strong> @ ${escapeHtml(ev.server_id)}</div>`;
      const open = () => attachSession(ev.session_id);
      li.addEventListener("click", open);
      li.addEventListener("keydown", (e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          open();
        }
      });
      approvalList.appendChild(li);
    }
    approvalCount.textContent = String(pendingCount);
  }

  async function fetchServers() {
    const resp = await api("/api/servers");
    if (!resp.ok) return;
    const body = await resp.json();
    state.servers = body.servers || [];
    if (state.selectedServerID && !state.servers.some((s) => s.server_id === state.selectedServerID)) {
      state.selectedServerID = "";
    }
    if (!state.selectedServerID && state.servers.length) {
      state.selectedServerID = state.servers[0].server_id;
    }
    if (maybeRedirectWindowsServerToChat(getSelectedServer())) return;
    renderServers();
  }

  async function fetchSessions() {
    const q = state.selectedServerID ? `?server_id=${encodeURIComponent(state.selectedServerID)}` : "";
    const resp = await api(`/api/sessions${q}`);
    if (!resp.ok) return;
    state.sessions = (await resp.json()).sessions || [];
    renderSessions();
    if (state.requestedSessionID && !state.selectedSessionID) {
      const target = state.sessions.find((s) => s.session_id === state.requestedSessionID);
      if (target) {
        state.requestedSessionID = "";
        attachSession(target.session_id);
      }
    }
  }

  async function refreshAll() {
    await fetchServers();
    await fetchSessions();
  }

  function attachSession(sessionID) {
    if (!sessionID) return;
    state.pendingFirstOutputSessionID = sessionID;
    state.selectedSessionID = sessionID;
    renderSessions();
    terminal.resetForSession(sessionID);
    sendWS({ type: "attach", data: { session_id: sessionID, since_seq: 0 } });
    terminal.sendResize();
    sidebar.closeSidebarOnMobile();
  }

  async function deleteSession(session) {
    if (!session?.session_id) return;
    if (session.status === "running") return alert("cannot delete running session");
    if (!window.confirm(`Delete session ${session.session_id.slice(0, 8)}?`)) return;
    const resp = await api(`/api/sessions/${encodeURIComponent(session.session_id)}`, { method: "DELETE" });
    if (!resp.ok) return alert(await resp.text());

    if (state.selectedSessionID === session.session_id) {
      state.selectedSessionID = "";
      state.pendingFirstOutputSessionID = "";
      terminal.clearSession();
    }
    for (const [eventID, approval] of state.approvals.entries()) {
      if (approval.session_id === session.session_id) state.approvals.delete(eventID);
    }
    renderApprovals();
    await fetchSessions();
  }

  async function switchSessionToChat(source) {
    if (!source?.session_id) return;
    const serverID = source.server_id || state.selectedServerID;
    if (!serverID) return alert("select a server first");
    const cwd = (source.cwd || "").trim();
    if (!cwd) return alert("cwd is required");
    const sessionID = source.session_id;
    const delResp = await api(`/api/sessions/${encodeURIComponent(sessionID)}`, { method: "DELETE" });
    if (!delResp.ok) return alert(await delResp.text());
    const createBody = {
      session_id: sessionID,
      server_id: serverID,
      session_type: "chat",
      cwd,
      env: parseEnv(envInput.value),
    };
    const resp = await api("/api/sessions", { method: "POST", body: JSON.stringify(createBody) });
    if (!resp.ok) return alert(formatSessionCreateError(await resp.text()));
    state.selectedServerID = serverID;
    window.location.href = `/chat?server_id=${encodeURIComponent(serverID)}&session_id=${encodeURIComponent(sessionID)}`;
  }

  function handleWS(msg) {
    if (msg.type === "term_out" && msg.session_id === state.selectedSessionID && msg.data_b64) {
      terminal.writeOutput(msg, state.pendingFirstOutputSessionID, () => {
        state.pendingFirstOutputSessionID = "";
        terminal.markLoaded(msg.session_id);
      });
      return;
    }

    if (msg.type === "event" && msg.data) {
      const ev = msg.data;
      if (ev.kind === "approval_needed") {
        state.approvals.set(ev.event_id, ev);
        renderApprovals();
      }
      return;
    }

    if (msg.type === "session_update" && msg.data) {
      const data = msg.data;
      if (!data.awaiting_approval) {
        for (const ev of state.approvals.values()) {
          if (ev.session_id === data.session_id && !ev.resolved) {
            ev.resolved = true;
            state.approvals.set(ev.event_id, ev);
          }
        }
        renderApprovals();
      }
      fetchSessions();
    }
  }

  function applyUIToken(token) {
    state.token = token || "";
    tokenInput.value = state.token;
    localStorage.setItem("ui_token", state.token);
    wsClient.reconnect();
    refreshAll();
  }

  async function createNewSession() {
    if (!state.selectedServerID) return alert("select a server first");
    if (maybeRedirectWindowsServerToChat(getSelectedServer())) return;

    const cwd = cwdInput.value.trim();
    if (!cwd) return alert("cwd is required");

    const customSessionID = sessionIDInput.value.trim().toLowerCase();
    const term = terminal.getTerm();
    const body = {
      server_id: state.selectedServerID,
      cwd,
      env: parseEnv(envInput.value),
      cols: term.cols,
      rows: term.rows,
    };
    if (customSessionID) body.session_id = customSessionID;

    const resp = await api("/api/sessions", { method: "POST", body: JSON.stringify(body) });
    if (!resp.ok) return alert(formatSessionCreateError(await resp.text()));

    const session = await resp.json();
    await fetchSessions();
    attachSession(session.session_id);
  }

  terminal.init();
  terminal.bindToolbarKeys();
  sidebar.mount();

  tokenInput.value = state.token;
  saveTokenBtn.addEventListener("click", () => applyUIToken(tokenInput.value.trim()));
  document.getElementById("refreshServersBtn").addEventListener("click", fetchServers);
  document.getElementById("refreshSessionsBtn").addEventListener("click", fetchSessions);
  document.getElementById("newSessionBtn").addEventListener("click", createNewSession);

  if (localStorage.getItem("ui_token")) {
    wsClient.connect();
    refreshAll();
  }
}
