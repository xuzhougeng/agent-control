import { WINDOWS_PTY_UNSUPPORTED_ERROR, escapeHtml, parseEnv, customConfirm } from "../shared/utils.js";
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
 * @param {HTMLElement} ctx.noticeList
 * @param {HTMLElement} ctx.noticeCount
 * @param {HTMLElement} ctx.instanceHistoryList
 * @param {HTMLElement} ctx.instanceHistoryCount
 * @param {HTMLElement} ctx.notificationToasts
 * @param {HTMLElement} ctx.cwdInput
 * @param {HTMLElement} ctx.sessionIDInput
 * @param {HTMLElement} ctx.envInput
 * @param {HTMLElement} ctx.permModeInput
 * @param {HTMLElement} ctx.permAllowedInput
 * @param {HTMLElement} ctx.permDisallowedInput
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
      // cc-agent (chat-mode) approvals carry an agent_request_id and a
      // pre-formatted prompt_excerpt like "[recursive rm] rm -rf /tmp/x".
      // For these we surface inline Approve/Reject buttons so the operator
      // can decide without having to type into a terminal.
      const isCredential = ev.kind === "credential_needed";
      const isAgentApproval = !!ev.agent_request_id && !isCredential;
      const promptText = ev.prompt_excerpt ? escapeHtml(ev.prompt_excerpt) : "";
      const promptHtml = promptText ? `<div class="approval-item-prompt"><code>${promptText}</code></div>` : "";
      const secretLabel = isCredential
        ? `<div class="approval-item-subtle">secret ${escapeHtml(ev.secret_name || "password")}</div>`
        : "";
      const buttonsHtml = isCredential
        ? `<div class="approval-item-secret">
             <input type="password" autocomplete="new-password" data-secret-input placeholder="Password" aria-label="Secret for ${escapeHtml(ev.secret_name || "credential")}">
           </div>
           <div class="approval-item-actions">
             <button type="button" data-action="credential_submit" class="approval-btn approval-btn-approve">Submit</button>
             <button type="button" data-action="credential_reject" class="approval-btn approval-btn-reject">Deny</button>
           </div>`
        : isAgentApproval
        ? `<div class="approval-item-actions">
             <button type="button" data-action="approve" class="approval-btn approval-btn-approve">Approve</button>
             <button type="button" data-action="reject" class="approval-btn approval-btn-reject">Reject</button>
           </div>`
        : "";
      li.innerHTML = `
        <div><strong>${escapeHtml(ev.session_id.slice(0, 8))}</strong> @ ${escapeHtml(ev.server_id)}</div>
        <div class="approval-item-subtle">${instanceText}</div>
        ${secretLabel}
        ${promptHtml}
        ${buttonsHtml}
      `;
      const open = () => attachSession(ev.session_id);
      li.addEventListener("click", (e) => {
        if (e.target.closest("button") || e.target.closest("input")) return;
        open();
      });
      li.addEventListener("keydown", (e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          open();
        }
      });
      const approveBtn = li.querySelector('[data-action="approve"]');
      const rejectBtn = li.querySelector('[data-action="reject"]');
      const submitBtn = li.querySelector('[data-action="credential_submit"]');
      const denyBtn = li.querySelector('[data-action="credential_reject"]');
      const secretInput = li.querySelector("[data-secret-input]");
      const decide = (kind, extra) => {
        // Make sure the UI is attached to this session so server-side
        // sub.AttachedSession is populated; cc-control falls back to it
        // for actions that omit session_id.
        attachSession(ev.session_id);
        const data = { kind, event_id: ev.event_id, ...(extra || {}) };
        ctx.wsClient.send({
          type: "action",
          session_id: ev.session_id,
          data,
        });
        if (secretInput) secretInput.value = "";
      };
      if (approveBtn) approveBtn.addEventListener("click", () => decide("approve"));
      if (rejectBtn) rejectBtn.addEventListener("click", () => decide("reject"));
      if (submitBtn) {
        submitBtn.addEventListener("click", () => {
          const secret = secretInput ? secretInput.value : "";
          if (!secret) {
            if (secretInput) secretInput.focus();
            return;
          }
          decide("credential_submit", { secret });
        });
      }
      if (denyBtn) denyBtn.addEventListener("click", () => decide("credential_reject"));
      if (secretInput) {
        secretInput.addEventListener("keydown", (e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            submitBtn?.click();
          }
        });
      }
      ctx.approvalList.appendChild(li);
    }
    ctx.approvalCount.textContent = String(pendingCount);
  }

  function formatNoticeTime(tsMS) {
    if (!tsMS) return "-";
    try {
      return new Date(tsMS).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
    } catch {
      return "-";
    }
  }

  function noticeLevelBadgeClass(level) {
    switch (String(level || "").toLowerCase()) {
      case "success":
        return "badge badge-notice-success";
      case "warning":
        return "badge badge-notice-warning";
      case "error":
        return "badge badge-notice-error";
      default:
        return "badge badge-notice-info";
    }
  }

  function trimNotificationState() {
    const values = Array.from(ctx.state.notifications.values()).sort((a, b) => (b.ts_ms || 0) - (a.ts_ms || 0));
    const capped = values.slice(0, 100);
    ctx.state.notifications = new Map(capped.map((ev) => [ev.notification_id, ev]));
  }

  function renderNotifications() {
    if (!ctx.noticeList || !ctx.noticeCount) return;
    ctx.noticeList.innerHTML = "";
    const values = Array.from(ctx.state.notifications.values()).sort((a, b) => (b.ts_ms || 0) - (a.ts_ms || 0));
    ctx.noticeCount.textContent = String(values.length);
    if (!values.length) {
      const li = document.createElement("li");
      li.className = "notice-item";
      li.textContent = "No notifications yet";
      ctx.noticeList.appendChild(li);
      return;
    }

    for (const ev of values) {
      const li = document.createElement("li");
      li.className = "notice-item";
      const sessionID = ev.session_id ? String(ev.session_id) : "";
      const hasSession = sessionID !== "";
      if (hasSession) {
        li.classList.add("is-clickable");
        li.tabIndex = 0;
        li.setAttribute("role", "button");
        li.setAttribute("aria-label", `Open session ${sessionID.slice(0, 8)} from notification`);
      }
      const level = String(ev.level || "info").toLowerCase();
      const title = String(ev.title || "").trim() || "Notification";
      const metaBits = [];
      if (ev.source) metaBits.push(String(ev.source));
      metaBits.push(formatNoticeTime(ev.ts_ms));
      if (ev.server_id) metaBits.push(`srv ${String(ev.server_id)}`);
      if (sessionID) metaBits.push(`sess ${sessionID.slice(0, 8)}`);
      li.innerHTML = `
        <div class="notice-item-head">
          <span class="notice-item-title">${escapeHtml(title)}</span>
          <span class="${noticeLevelBadgeClass(level)}">${escapeHtml(level)}</span>
        </div>
        <div class="notice-item-message">${escapeHtml(String(ev.message || ""))}</div>
        <div class="notice-item-meta">${escapeHtml(metaBits.join(" · "))}</div>
      `;
      if (hasSession) {
        const open = () => attachSession(sessionID);
        li.addEventListener("click", open);
        li.addEventListener("keydown", (e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            open();
          }
        });
      }
      ctx.noticeList.appendChild(li);
    }
  }

  function showNotificationToast(ev) {
    if (!ctx.notificationToasts) return;
    const levelRaw = String(ev.level || "info").toLowerCase();
    const level = ["info", "success", "warning", "error"].includes(levelRaw) ? levelRaw : "info";
    const title = String(ev.title || "").trim() || "Notification";
    const toast = document.createElement("div");
    toast.className = `notification-toast level-${level}`;
    const metaBits = [];
    if (ev.source) metaBits.push(String(ev.source));
    metaBits.push(formatNoticeTime(ev.ts_ms));
    toast.innerHTML = `
      <div class="notification-toast-title">${escapeHtml(title)}</div>
      <div class="notification-toast-message">${escapeHtml(String(ev.message || ""))}</div>
      <div class="notification-toast-meta">${escapeHtml(metaBits.join(" · "))}</div>
    `;
    ctx.notificationToasts.appendChild(toast);
    while (ctx.notificationToasts.children.length > 4) {
      ctx.notificationToasts.removeChild(ctx.notificationToasts.firstElementChild);
    }
    window.setTimeout(() => {
      toast.remove();
    }, 6000);
  }

  function pushNotification(rawEvent) {
    const notificationID = String(rawEvent?.notification_id || "").trim();
    if (!notificationID) return;
    const existed = ctx.state.notifications.has(notificationID);
    ctx.state.notifications.set(notificationID, rawEvent);
    trimNotificationState();
    renderNotifications();
    if (!existed) {
      showNotificationToast(rawEvent);
    }
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
    ctx.chat.renderPermissionBar();
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
    if (!await customConfirm(`Delete session ${session.session_id.slice(0, 8)}?`, { danger: true, confirmLabel: "删除", cancelLabel: "取消" })) return;
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

  async function applyPermissions() {
    const session = ctx.getSelectedSession();
    if (!session || session.session_type !== "chat") return;
    if (!await customConfirm("将重启当前 chat 会话以应用新权限，继续？", { confirmLabel: "应用", cancelLabel: "取消" })) return;

    const permMode = ctx.permModeInput ? ctx.permModeInput.value : "";
    const allowed = ctx.permAllowedInput ? ctx.permAllowedInput.value.trim() : "";
    const disallowed = ctx.permDisallowedInput ? ctx.permDisallowedInput.value.trim() : "";

    const baseEnv = parseEnv(ctx.envInput.value);
    if (permMode) baseEnv["CC_CLAUDE_PERMISSION_MODE"] = permMode;
    if (allowed) baseEnv["CC_CLAUDE_ALLOWED_TOOLS"] = allowed;
    else delete baseEnv["CC_CLAUDE_ALLOWED_TOOLS"];
    if (disallowed) baseEnv["CC_CLAUDE_DISALLOWED_TOOLS"] = disallowed;
    else delete baseEnv["CC_CLAUDE_DISALLOWED_TOOLS"];

    const body = { session_type: "chat", env: baseEnv };
    const resp = await ctx.api(
      `/api/sessions/${encodeURIComponent(session.session_id)}/switch`,
      { method: "POST", body: JSON.stringify(body) },
    );
    if (!resp.ok) return alert(await resp.text());
    await fetchSessions();
    await attachSession(session.session_id);
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
    applyPermissions,
    renderServers,
    renderSessions,
    renderInstanceHistory,
    renderApprovals,
    renderNotifications,
    pushNotification,
    openView,
    attachSelectedView,
  };
}
