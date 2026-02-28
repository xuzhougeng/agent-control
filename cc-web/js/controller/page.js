import { createUIApi } from "../shared/http.js";
import { createSidebarController } from "../shared/sidebar.js";
import { createWSClient, setWSBadge } from "../shared/ws.js";
import { createTerminalController } from "./terminal.js";
import { createWorkspaceShell } from "../shared/workspace-shell.js";
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
    switchingSessionID: "",
    pendingFirstOutputSessionID: "",
    approvals: new Map(),
    sessions: [],
    servers: [],
    instanceHistory: [],
  };

  const api = createUIApi(() => state.token);

  const tokenInput = document.getElementById("tokenInput");
  const saveTokenBtn = document.getElementById("saveTokenBtn");
  const serversList = document.getElementById("serversList");
  const sessionsList = document.getElementById("sessionsList");
  const approvalList = document.getElementById("approvalList");
  const approvalCount = document.getElementById("approvalCount");
  const approvalDetails = document.getElementById("approvalDetails");
  const instanceHistoryList = document.getElementById("instanceHistoryList");
  const instanceHistoryCount = document.getElementById("instanceHistoryCount");
  const cwdInput = document.getElementById("cwdInput");
  const sessionIDInput = document.getElementById("sessionIdInput");
  const envInput = document.getElementById("envInput");

  function sendWS(msg) {
    return wsClient.send(msg);
  }

  function getSelectedSession() {
    return state.sessions.find((s) => s.session_id === state.selectedSessionID) || null;
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

  const workspace = createWorkspaceShell({
    viewMode: "pty",
    getSelectedSession,
    onOpenTerminalView: (session) => attachSession(session.session_id),
    onOpenChatView: (session) => openChatView(session),
    onSwitchMode: (session) => switchSessionToTerminal(session),
    onCopySession: async (session) => {
      try {
        await navigator.clipboard.writeText(session.session_id || "");
      } catch {
        alert("copy failed");
      }
    },
  });

  const wsClient = createWSClient({
    getToken: () => state.token,
    shouldConnect: () => true,
    setStatus: setWSBadge,
    onOpen: ({ send }) => {
      state.ws = wsClient.getSocket();
      if (state.selectedSessionID && (getSelectedSession()?.session_type || "pty") === "pty") {
        send({ type: "attach", data: { session_id: state.selectedSessionID, since_seq: 0 } });
        terminal.sendResize();
      }
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
      selectedSessionID: state.selectedSessionID,
      renderItem: (s, isSelected) => {
        const isSwitching = state.switchingSessionID === s.session_id;
        const li = document.createElement("li");
        li.classList.add("session-item");
        if (isSelected) li.classList.add("selected");
        const statusBadge = s.status === "running" ? "badge badge-running" : "badge";
        const canDelete = s.status !== "running" && !isSwitching;
        const approvalClass = s.awaiting_approval ? "badge-pending" : "badge-muted";
        const modeLabel = s.session_type === "chat" ? "chat" : "pty";
        li.innerHTML = `
          <div class="session-main">
            <strong class="session-id">${escapeHtml(s.session_id.slice(0, 8))}</strong>
            <div class="session-badges">
              <span class="badge">${escapeHtml(modeLabel)}</span>
              <span class="${statusBadge}">${escapeHtml(s.status)}</span>
              <span class="badge ${approvalClass}">${s.awaiting_approval ? "approval" : "normal"}</span>
            </div>
          </div>
          <div class="session-sub">${escapeHtml(s.cwd || "-")}</div>
          <div class="session-detail"><span>active ${(s.active_instance_id || "-").slice(0, 8)}</span></div>
          ${s.exit_reason
            ? `<div class="session-detail"><span>reason ${escapeHtml(s.exit_reason)}</span></div>`
            : ""}
          <div class="session-actions">
            <button type="button" data-action="delete" class="btn-danger" ${canDelete ? "" : "disabled"}>Delete</button>
          </div>
        `;
        li.querySelector('[data-action="delete"]').addEventListener("click", async (e) => {
          e.stopPropagation();
          await deleteSession(s);
        });
        li.addEventListener("click", () => attachSession(s.session_id));
        return li;
      },
    });
    workspace.render(getSelectedSession());
  }

  function renderInstanceHistory() {
    if (!instanceHistoryList || !instanceHistoryCount) return;
    instanceHistoryList.innerHTML = "";
    const session = state.sessions.find((s) => s.session_id === state.selectedSessionID);
    const items = state.instanceHistory || [];
    instanceHistoryCount.textContent = String(items.length);
    if (!state.selectedSessionID) {
      const li = document.createElement("li");
      li.className = "session-item";
      li.textContent = "Select a session";
      instanceHistoryList.appendChild(li);
      return;
    }
    if (!items.length) {
      const li = document.createElement("li");
      li.className = "session-item";
      li.textContent = "No instances";
      instanceHistoryList.appendChild(li);
      return;
    }
    for (const inst of items) {
      const li = document.createElement("li");
      li.className = "session-item";
      if (session && session.active_instance_id === inst.instance_id) {
        li.classList.add("selected");
      }
      const statusBadge = inst.status === "running" ? "badge badge-running" : "badge";
      const createdAt = inst.created_at_ms ? new Date(inst.created_at_ms).toLocaleString() : "-";
      li.innerHTML = `
        <div class="session-main">
          <strong class="session-id">${escapeHtml((inst.instance_id || "").slice(0, 8) || "-")}</strong>
          <div class="session-badges">
            <span class="badge">${escapeHtml(inst.session_type || "-")}</span>
            <span class="${statusBadge}">${escapeHtml(inst.status || "-")}</span>
          </div>
        </div>
        <div class="session-sub">${escapeHtml(createdAt)}</div>
        ${inst.exit_reason ? `<div class="session-detail"><span>reason ${escapeHtml(inst.exit_reason)}</span></div>` : ""}
      `;
      instanceHistoryList.appendChild(li);
    }
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
      const instanceText = ev.instance_id ? `instance ${escapeHtml(ev.instance_id.slice(0, 8))}` : "instance -";
      li.innerHTML = `
        <div><strong>${escapeHtml(ev.session_id.slice(0, 8))}</strong> @ ${escapeHtml(ev.server_id)}</div>
        <div class="approval-item-subtle">${instanceText}</div>
      `;
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
    renderInstanceHistory();
    if (state.selectedSessionID) {
      const active = getSelectedSession();
      if (active && (active.session_type || "pty") === "pty") {
        terminal.setSessionInfo(active.session_id, active.active_instance_id || "", state.pendingFirstOutputSessionID === active.session_id);
      } else if (active) {
        terminal.clearSession();
      }
    }
    if (state.requestedSessionID && !state.selectedSessionID) {
      const target = state.sessions.find((s) => s.session_id === state.requestedSessionID);
      if (target) {
        state.requestedSessionID = "";
        attachSession(target.session_id);
      }
    }
    if (state.selectedSessionID && !state.sessions.some((s) => s.session_id === state.selectedSessionID)) {
      state.selectedSessionID = "";
      state.pendingFirstOutputSessionID = "";
      state.instanceHistory = [];
      terminal.clearSession();
      renderInstanceHistory();
      workspace.render(null);
    }
  }

  async function refreshAll() {
    await fetchServers();
    await fetchSessions();
  }

  function attachSession(sessionID) {
    if (!sessionID) return;
    state.selectedSessionID = sessionID;
    renderSessions();
    fetchSessionInstances(sessionID);
    const session = getSelectedSession();
    if (session && (session.session_type || "pty") === "pty") {
      state.pendingFirstOutputSessionID = sessionID;
      terminal.resetForSession(sessionID);
      sendWS({ type: "attach", data: { session_id: sessionID, since_seq: 0 } });
      terminal.sendResize();
    } else {
      state.pendingFirstOutputSessionID = "";
      terminal.clearSession();
    }
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
      state.instanceHistory = [];
      renderInstanceHistory();
      terminal.clearSession();
      workspace.render(null);
    }
    for (const [eventID, approval] of state.approvals.entries()) {
      if (approval.session_id === session.session_id) state.approvals.delete(eventID);
    }
    renderApprovals();
    await fetchSessions();
  }

  async function fetchSessionInstances(sessionID) {
    if (!sessionID) {
      state.instanceHistory = [];
      renderInstanceHistory();
      return;
    }
    const resp = await api(`/api/sessions/${encodeURIComponent(sessionID)}/instances`);
    if (!resp.ok) return;
    if (state.selectedSessionID !== sessionID) return;
    state.instanceHistory = (await resp.json()).instances || [];
    renderInstanceHistory();
  }

  async function switchSessionToChat(source) {
    if (!source?.session_id) return;
    if (state.switchingSessionID) return;
    const serverID = source.server_id || state.selectedServerID;
    if (!serverID) return alert("select a server first");
    const sessionID = source.session_id;
    if ((source.session_type || "pty") === "chat") {
      openChatView(source);
      return;
    }
    state.switchingSessionID = sessionID;
    renderSessions();
    try {
      const switchBody = {
        session_type: "chat",
        env: parseEnv(envInput.value),
      };
      const resp = await api(`/api/sessions/${encodeURIComponent(sessionID)}/switch`, {
        method: "POST",
        body: JSON.stringify(switchBody),
      });
      if (!resp.ok) return alert(formatSessionCreateError(await resp.text()));
      openChatView({ ...source, server_id: serverID });
    } finally {
      state.switchingSessionID = "";
      renderSessions();
    }
  }

  async function switchSessionToTerminal(source) {
    if (!source?.session_id) return;
    if ((source.session_type || "pty") === "pty") {
      attachSession(source.session_id);
      return;
    }
    if (state.switchingSessionID) return;
    const sessionID = source.session_id;
    state.switchingSessionID = sessionID;
    renderSessions();
    try {
      const term = terminal.getTerm();
      const resp = await api(`/api/sessions/${encodeURIComponent(sessionID)}/switch`, {
        method: "POST",
        body: JSON.stringify({
          session_type: "pty",
          env: parseEnv(envInput.value),
          cols: term.cols,
          rows: term.rows,
        }),
      });
      if (!resp.ok) return alert(formatSessionCreateError(await resp.text()));
      await fetchSessions();
      attachSession(sessionID);
    } finally {
      state.switchingSessionID = "";
      renderSessions();
    }
  }

  function openChatView(source) {
    const serverID = source?.server_id || state.selectedServerID || "";
    const sessionID = source?.session_id || state.selectedSessionID || "";
    if (!sessionID) return;
    const query = new URLSearchParams();
    if (serverID) query.set("server_id", serverID);
    query.set("session_id", sessionID);
    window.location.href = `/chat?${query.toString()}`;
  }

  function handleWS(msg) {
    if (msg.type === "term_out" && msg.session_id === state.selectedSessionID && msg.data_b64) {
      terminal.writeOutput(msg, state.pendingFirstOutputSessionID, () => {
        state.pendingFirstOutputSessionID = "";
        terminal.markLoaded(msg.session_id, msg.instance_id || "");
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
      if (data.session_id === state.selectedSessionID) {
        fetchSessionInstances(state.selectedSessionID);
      }
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
  renderInstanceHistory();
  workspace.render(null);

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
