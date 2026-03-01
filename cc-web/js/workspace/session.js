import { WINDOWS_PTY_UNSUPPORTED_ERROR, escapeHtml, parseEnv } from "../shared/utils.js";
import { renderServerList } from "../shared/server-list.js";
import { renderSessionList } from "../shared/session-list.js";

/**
 * @param {object} ctx
 * @param {object} ctx.state
 * @param {object} ctx.api
 * @param {object} ctx.terminal
 * @param {object} ctx.workspace
 * @param {object} ctx.sidebar
 * @param {function} ctx.sendWS
 * @param {function} ctx.syncQuery
 * @param {function} ctx.updateViewVisibility
 * @param {function} ctx.getSelectedSession
 * @param {function} ctx.getServerByID
 * @param {function} ctx.getSelectedServer
 * @param {object} ctx.chat
 * @param {HTMLElement} ctx.serversList
 * @param {HTMLElement} ctx.sessionsList
 * @param {HTMLElement} ctx.approvalList
 * @param {HTMLElement} ctx.approvalCount
 * @param {HTMLElement} ctx.instanceHistoryList
 * @param {HTMLElement} ctx.instanceHistoryCount
 * @param {HTMLElement} ctx.cwdInput
 * @param {HTMLElement} ctx.sessionIDInput
 * @param {HTMLElement} ctx.envInput
 */
export function createSessionController(ctx) {
  function isWindowsServer(server) {
    return String((server && server.os) || "").toLowerCase() === "windows";
  }

  function ensureSupportedViewForServer(server) {
    if (!isWindowsServer(server) || ctx.state.currentView !== "pty") return false;
    ctx.state.currentView = "chat";
    ctx.updateViewVisibility();
    ctx.syncQuery();
    alert("Selected server is Windows. PTY is not supported yet; switched to Chat view.");
    return true;
  }

  function formatSessionCreateError(rawText) {
    const msg = String(rawText || "").trim();
    if (msg.includes(WINDOWS_PTY_UNSUPPORTED_ERROR)) {
      return "PTY is not supported on Windows yet; switch the unified workspace to Chat view.";
    }
    if (msg.includes("invalid session_id")) {
      return "session_id must be a valid UUID.";
    }
    if (msg.includes("session_id already exists")) {
      return "session_id already exists.";
    }
    if (msg.includes("not supported on Windows")) {
      return "PTY is not supported on Windows yet.";
    }
    return msg || "request failed";
  }

  function renderServers() {
    renderServerList(ctx.serversList, ctx.state.servers, ctx.state.selectedServerID, async (server) => {
      ctx.state.selectedServerID = server.server_id;
      renderServers();
      ensureSupportedViewForServer(server);
      await fetchSessions();
      ctx.syncQuery();
    });
  }

  function renderSessions() {
    renderSessionList({
      listEl: ctx.sessionsList,
      sessions: ctx.state.sessions,
      selectedSessionID: ctx.state.selectedSessionID,
      renderItem: (s, isSelected) => {
        const isSwitching = ctx.state.switchingSessionID === s.session_id;
        const li = document.createElement("li");
        li.classList.add("session-item", "session-item-nav");
        if (isSelected) li.classList.add("selected");
        const statusBadge = s.status === "running" ? "badge badge-running" : "badge";
        const canDelete = s.status !== "running" && !isSwitching;
        const approvalClass = s.awaiting_approval ? "badge-pending" : "badge-muted";
        const modeLabel = s.session_type === "chat" ? "chat" : "pty";
        const shortSessionID = escapeHtml(s.session_id.slice(0, 8));
        const shortInstanceID = escapeHtml((s.active_instance_id || "-").slice(0, 8));
        li.innerHTML = `
          <div class="session-main session-main-nav">
            <div class="session-title-wrap">
              <span class="session-status-dot ${s.status === "running" ? "is-running" : ""}"></span>
              <strong class="session-id">${shortSessionID}</strong>
            </div>
            <div class="session-badges">
              <span class="badge">${escapeHtml(modeLabel)}</span>
              <span class="${statusBadge}">${escapeHtml(s.status)}</span>
              ${s.awaiting_approval ? `<span class="badge ${approvalClass}">approval</span>` : ""}
            </div>
          </div>
          <div class="session-sub session-sub-nav">${escapeHtml(s.cwd || "-")}</div>
          <div class="session-detail session-detail-nav">
            <span>instance ${shortInstanceID}</span>
            <span>${escapeHtml(modeLabel)} active</span>
          </div>
          ${s.exit_reason
            ? `<div class="session-detail session-detail-nav"><span>reason ${escapeHtml(s.exit_reason)}</span></div>`
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
    ctx.workspace.render(ctx.getSelectedSession());
  }

  function renderInstanceHistory() {
    if (!ctx.instanceHistoryList || !ctx.instanceHistoryCount) return;
    ctx.instanceHistoryList.innerHTML = "";
    const session = ctx.getSelectedSession();
    const items = ctx.state.instanceHistory || [];
    ctx.instanceHistoryCount.textContent = String(items.length);
    if (!ctx.state.selectedSessionID) {
      const li = document.createElement("li");
      li.className = "session-item instance-item";
      li.textContent = "Select a session";
      ctx.instanceHistoryList.appendChild(li);
      return;
    }
    if (!items.length) {
      const li = document.createElement("li");
      li.className = "session-item instance-item";
      li.textContent = "No instances";
      ctx.instanceHistoryList.appendChild(li);
      return;
    }
    for (const inst of items) {
      const li = document.createElement("li");
      li.className = "session-item instance-item";
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
      ctx.instanceHistoryList.appendChild(li);
    }
  }

  function renderApprovals() {
    if (!ctx.approvalList || !ctx.approvalCount) return;
    ctx.approvalList.innerHTML = "";
    const values = Array.from(ctx.state.approvals.values()).sort((a, b) => b.ts_ms - a.ts_ms);
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
      ctx.approvalList.appendChild(li);
    }
    ctx.approvalCount.textContent = String(pendingCount);
  }

  async function fetchServers() {
    const resp = await ctx.api("/api/servers");
    if (!resp.ok) return;
    const body = await resp.json();
    ctx.state.servers = body.servers || [];
    if (ctx.state.selectedServerID && !ctx.state.servers.some((s) => s.server_id === ctx.state.selectedServerID)) {
      ctx.state.selectedServerID = "";
    }
    if (!ctx.state.selectedServerID && ctx.state.servers.length) {
      ctx.state.selectedServerID = ctx.state.servers[0].server_id;
    }
    ensureSupportedViewForServer(ctx.getSelectedServer());
    renderServers();
  }

  async function fetchSessions() {
    const q = ctx.state.selectedServerID ? `?server_id=${encodeURIComponent(ctx.state.selectedServerID)}` : "";
    const resp = await ctx.api(`/api/sessions${q}`);
    if (!resp.ok) return;
    ctx.state.sessions = (await resp.json()).sessions || [];
    renderSessions();
    renderInstanceHistory();

    if (ctx.state.requestedSessionID && !ctx.state.selectedSessionID) {
      const target = ctx.state.sessions.find((s) => s.session_id === ctx.state.requestedSessionID);
      if (target) {
        ctx.state.requestedSessionID = "";
        await attachSession(target.session_id);
        return;
      }
    }

    if (ctx.state.selectedSessionID && !ctx.state.sessions.some((s) => s.session_id === ctx.state.selectedSessionID)) {
      ctx.state.selectedSessionID = "";
      ctx.state.pendingFirstOutputSessionID = "";
      ctx.state.instanceHistory = [];
      ctx.state.chatMessages = [];
      ctx.state.pendingScreenshots = [];
      ctx.terminal.clearSession();
      renderInstanceHistory();
      ctx.chat.renderChatMessages();
      ctx.chat.renderPendingScreenshots();
      ctx.workspace.render(null);
      ctx.syncQuery();
      return;
    }

    const active = ctx.getSelectedSession();
    if (active && (active.session_type || "pty") === "pty") {
      ctx.terminal.setSessionInfo(active.session_id, active.active_instance_id || "", ctx.state.pendingFirstOutputSessionID === active.session_id);
    } else if (!active || ctx.state.currentView === "pty") {
      ctx.terminal.clearSession();
    }
    ctx.workspace.render(active);
  }

  async function refreshAll() {
    await fetchServers();
    await fetchSessions();
  }

  async function fetchSessionInstances(sessionID) {
    if (!sessionID) {
      ctx.state.instanceHistory = [];
      renderInstanceHistory();
      return;
    }
    const resp = await ctx.api(`/api/sessions/${encodeURIComponent(sessionID)}/instances`);
    if (!resp.ok) return;
    if (ctx.state.selectedSessionID !== sessionID) return;
    ctx.state.instanceHistory = (await resp.json()).instances || [];
    renderInstanceHistory();
  }

  async function loadChatHistory(sessionID) {
    try {
      const resp = await ctx.api(`/api/sessions/${encodeURIComponent(sessionID)}/chat`);
      if (!resp.ok) return;
      if (ctx.state.selectedSessionID !== sessionID) return;
      ctx.state.chatMessages = (await resp.json()).messages || [];
      ctx.chat.renderChatMessages();
    } catch (e) {
      console.error("[workspace] failed to load chat history", e);
    }
  }

  function attachSelectedView(send = ctx.sendWS) {
    const session = ctx.getSelectedSession();
    if (!session) return;
    if (ctx.state.currentView === "pty") {
      if ((session.session_type || "pty") !== "pty") {
        ctx.state.pendingFirstOutputSessionID = "";
        ctx.terminal.clearSession();
        return;
      }
      ctx.state.pendingFirstOutputSessionID = session.session_id;
      ctx.terminal.resetForSession(session.session_id);
      send({ type: "attach", data: { session_id: session.session_id, since_seq: 0 } });
      ctx.terminal.sendResize();
      return;
    }
    ctx.state.pendingFirstOutputSessionID = "";
    ctx.terminal.setSessionInfo(session.session_id, session.active_instance_id || "", false);
    if ((session.session_type || "pty") !== "chat") return;
    send({ type: "attach", data: { session_id: session.session_id, since_seq: 0 } });
  }

  async function syncSelectedSession() {
    const session = ctx.getSelectedSession();
    ctx.workspace.render(session);
    if (!session) {
      ctx.chat.renderChatMessages();
      ctx.chat.renderPendingScreenshots();
      renderInstanceHistory();
      ctx.terminal.clearSession();
      return;
    }
    fetchSessionInstances(session.session_id);
    if (ctx.state.currentView === "pty") {
      ctx.state.chatMessages = [];
      ctx.chat.renderChatMessages();
      attachSelectedView();
    } else {
      ctx.terminal.setSessionInfo(session.session_id, session.active_instance_id || "", false);
      attachSelectedView();
      if ((session.session_type || "pty") === "chat") {
        await loadChatHistory(session.session_id);
      } else {
        ctx.state.chatMessages = [];
        ctx.chat.renderChatMessages();
      }
    }
  }

  async function openView(view, sessionID = ctx.state.selectedSessionID) {
    ctx.state.currentView = view === "chat" ? "chat" : "pty";
    if (sessionID) ctx.state.selectedSessionID = sessionID;
    ctx.updateViewVisibility();
    ctx.syncQuery();
    renderSessions();
    await syncSelectedSession();
  }

  async function attachSession(sessionID) {
    if (!sessionID) return;
    ctx.state.selectedSessionID = sessionID;
    ctx.state.pendingScreenshots = [];
    ctx.state.pendingTurns = 0;
    ctx.chat.clearPendingSlowTimer();
    ctx.chat.setRunState("", "");
    ctx.chat.renderPendingScreenshots();
    renderSessions();
    ctx.syncQuery();
    await syncSelectedSession();
    ctx.sidebar.closeSidebarOnMobile();
  }

  async function deleteSession(session) {
    if (!session?.session_id) return;
    if (session.status === "running") return alert("cannot delete running session");
    if (!window.confirm(`Delete session ${session.session_id.slice(0, 8)}?`)) return;
    const resp = await ctx.api(`/api/sessions/${encodeURIComponent(session.session_id)}`, { method: "DELETE" });
    if (!resp.ok) return alert(await resp.text());

    if (ctx.state.selectedSessionID === session.session_id) {
      ctx.state.selectedSessionID = "";
      ctx.state.pendingFirstOutputSessionID = "";
      ctx.state.instanceHistory = [];
      ctx.state.chatMessages = [];
      ctx.state.pendingScreenshots = [];
      renderInstanceHistory();
      ctx.chat.renderChatMessages();
      ctx.chat.renderPendingScreenshots();
      ctx.terminal.clearSession();
      ctx.workspace.render(null);
      ctx.syncQuery();
    }
    for (const [eventID, approval] of ctx.state.approvals.entries()) {
      if (approval.session_id === session.session_id) ctx.state.approvals.delete(eventID);
    }
    renderApprovals();
    await fetchSessions();
  }

  async function switchSessionTo(targetView, source) {
    if (!source?.session_id) return;
    if (ctx.state.switchingSessionID) return;
    const serverID = source.server_id || ctx.state.selectedServerID;
    if (!serverID) return alert("select a server first");

    if (targetView === "pty" && isWindowsServer(ctx.getServerByID(serverID))) {
      alert("PTY is not supported on Windows yet.");
      return;
    }

    if ((source.session_type || "pty") === targetView) {
      await openView(targetView, source.session_id);
      return;
    }

    ctx.state.switchingSessionID = source.session_id;
    renderSessions();
    try {
      const body = {
        session_type: targetView,
        env: parseEnv(ctx.envInput.value),
      };
      if (targetView === "pty") {
        const term = ctx.terminal.getTerm();
        body.cols = term?.cols || 120;
        body.rows = term?.rows || 30;
      }
      const resp = await ctx.api(`/api/sessions/${encodeURIComponent(source.session_id)}/switch`, {
        method: "POST",
        body: JSON.stringify(body),
      });
      if (!resp.ok) return alert(formatSessionCreateError(await resp.text()));
      await fetchSessions();
      await openView(targetView, source.session_id);
    } finally {
      ctx.state.switchingSessionID = "";
      renderSessions();
    }
  }

  async function createNewSession() {
    if (!ctx.state.selectedServerID) return alert("select a server first");
    const server = ctx.getSelectedServer();
    ensureSupportedViewForServer(server);

    const cwd = ctx.cwdInput.value.trim();
    if (!cwd) return alert("cwd is required");

    const customSessionID = ctx.sessionIDInput.value.trim().toLowerCase();
    const body = {
      server_id: ctx.state.selectedServerID,
      cwd,
      env: parseEnv(ctx.envInput.value),
    };
    if (customSessionID) body.session_id = customSessionID;

    if (ctx.state.currentView === "chat") {
      body.session_type = "chat";
    } else {
      const term = ctx.terminal.getTerm();
      body.cols = term?.cols || 120;
      body.rows = term?.rows || 30;
    }

    const resp = await ctx.api("/api/sessions", { method: "POST", body: JSON.stringify(body) });
    if (!resp.ok) return alert(formatSessionCreateError(await resp.text()));

    const created = await resp.json();
    await fetchSessions();
    await attachSession(created.session_id);
  }

  return {
    fetchServers,
    fetchSessions,
    refreshAll,
    fetchSessionInstances,
    attachSession,
    deleteSession,
    switchSessionTo,
    createNewSession,
    renderServers,
    renderSessions,
    renderInstanceHistory,
    renderApprovals,
    openView,
    attachSelectedView,
  };
}
