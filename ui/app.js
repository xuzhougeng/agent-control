(() => {
  const b64ToBytes = (b64) => {
    const bin = atob(b64);
    const arr = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) arr[i] = bin.charCodeAt(i);
    return arr;
  };

  const bytesToB64 = (str) => {
    const bytes = new TextEncoder().encode(str);
    let bin = "";
    for (const b of bytes) bin += String.fromCharCode(b);
    return btoa(bin);
  };

  const state = {
    token: localStorage.getItem("ui_token") || "admin-dev-token",
    adminToken: localStorage.getItem("admin_token") || "admin-dev-token",
    sidebarPanel: localStorage.getItem("sidebar_panel") || "control",
    selectedServerID: "",
    selectedSessionID: "",
    pendingFirstOutputSessionID: "",
    ws: null,
    approvals: new Map(),
    sessions: [],
    servers: [],
    lastGeneratedToken: null,
    adminTokens: [],
  };

  const tokenInput = document.getElementById("tokenInput");
  const saveTokenBtn = document.getElementById("saveTokenBtn");
  const adminTokenInput = document.getElementById("adminTokenInput");
  const adminTypeSelect = document.getElementById("adminTypeSelect");
  const adminRoleSelect = document.getElementById("adminRoleSelect");
  const adminTenantInput = document.getElementById("adminTenantInput");
  const adminNameInput = document.getElementById("adminNameInput");
  const adminGenerateBtn = document.getElementById("adminGenerateBtn");
  const adminListTokensBtn = document.getElementById("adminListTokensBtn");
  const adminCopyTokenBtn = document.getElementById("adminCopyTokenBtn");
  const adminUseUiTokenBtn = document.getElementById("adminUseUiTokenBtn");
  const adminMessage = document.getElementById("adminMessage");
  const adminResult = document.getElementById("adminResult");
  const adminTokensList = document.getElementById("adminTokensList");
  const serversList = document.getElementById("serversList");
  const sessionsList = document.getElementById("sessionsList");
  const approvalList = document.getElementById("approvalList");
  const approvalCount = document.getElementById("approvalCount");
  const approvalDetails = document.getElementById("approvalDetails");
  const cwdInput = document.getElementById("cwdInput");
  const resumeInput = document.getElementById("resumeInput");
  const envInput = document.getElementById("envInput");
  const currentSessionLabel = document.getElementById("currentSessionLabel");
  const sidebarToggleBtn = document.getElementById("sidebarToggleBtn");
  const sidebarBackdrop = document.getElementById("sidebarBackdrop");
  const tabControlBtn = document.getElementById("tabControlBtn");
  const tabAdminBtn = document.getElementById("tabAdminBtn");
  const sidebarPanelControl = document.getElementById("sidebarPanelControl");
  const sidebarPanelAdmin = document.getElementById("sidebarPanelAdmin");
  const mobileMedia = window.matchMedia("(max-width: 900px)");
  let mobileKeyboardOpen = false;

  tokenInput.value = state.token;
  adminTokenInput.value = state.adminToken;

  const term = new Terminal({
    cursorBlink: true,
    convertEol: true,
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    fontSize: 14,
    lineHeight: 1.2,
    theme: { background: "#0b1020" },
  });
  const fitAddon = new FitAddon.FitAddon();
  term.loadAddon(fitAddon);
  term.open(document.getElementById("terminal"));
  fitAddon.fit();
  window.addEventListener("resize", () => { fitAddon.fit(); sendResize(); });
  new ResizeObserver(() => { fitAddon.fit(); sendResize(); }).observe(document.getElementById("terminal"));

  function isMobileViewport() {
    return mobileMedia.matches;
  }

  function syncTerminalLayout() {
    fitAddon.fit();
    sendResize();
  }

  function handleMobileViewportChange() {
    if (!isMobileViewport()) {
      mobileKeyboardOpen = false;
      return;
    }
    const vv = window.visualViewport;
    if (!vv) {
      return;
    }
    // On mobile, a large visual viewport height drop usually means keyboard opened.
    const keyboardNowOpen = (window.innerHeight - vv.height) > 120;
    if (keyboardNowOpen && !mobileKeyboardOpen) {
      setTimeout(() => {
        syncTerminalLayout();
        term.scrollToBottom();
        document.getElementById("terminal").scrollIntoView({ behavior: "smooth", block: "end" });
      }, 0);
    } else {
      syncTerminalLayout();
    }
    mobileKeyboardOpen = keyboardNowOpen;
  }

  function toggleSidebar(open) {
    const nextOpen = typeof open === "boolean" ? open : !document.body.classList.contains("sidebar-open");
    document.body.classList.toggle("sidebar-open", nextOpen);
    sidebarToggleBtn.setAttribute("aria-expanded", String(nextOpen));
    sidebarBackdrop.hidden = !nextOpen;
    setTimeout(syncTerminalLayout, 0);
  }

  function closeSidebarOnMobile() {
    if (isMobileViewport()) {
      toggleSidebar(false);
    }
  }

  function setSidebarPanel(panelName) {
    const panel = panelName === "admin" ? "admin" : "control";
    state.sidebarPanel = panel;
    localStorage.setItem("sidebar_panel", panel);

    const controlActive = panel === "control";
    tabControlBtn.classList.toggle("active", controlActive);
    tabAdminBtn.classList.toggle("active", !controlActive);
    tabControlBtn.setAttribute("aria-selected", String(controlActive));
    tabAdminBtn.setAttribute("aria-selected", String(!controlActive));
    sidebarPanelControl.classList.toggle("active", controlActive);
    sidebarPanelAdmin.classList.toggle("active", !controlActive);
  }

  function initializeApprovalDetails() {
    const saved = localStorage.getItem("approval_collapsed");
    if (saved === "1") {
      approvalDetails.removeAttribute("open");
      return;
    }
    if (saved === "0") {
      approvalDetails.setAttribute("open", "");
      return;
    }
    if (isMobileViewport()) {
      approvalDetails.removeAttribute("open");
      return;
    }
    approvalDetails.setAttribute("open", "");
  }

  initializeApprovalDetails();
  toggleSidebar(false);
  setSidebarPanel(state.sidebarPanel);

  sidebarToggleBtn.addEventListener("click", () => toggleSidebar());
  tabControlBtn.addEventListener("click", () => setSidebarPanel("control"));
  tabAdminBtn.addEventListener("click", () => setSidebarPanel("admin"));
  sidebarBackdrop.addEventListener("click", () => toggleSidebar(false));
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && document.body.classList.contains("sidebar-open")) {
      toggleSidebar(false);
    }
  });
  mobileMedia.addEventListener("change", () => {
    if (!isMobileViewport()) {
      toggleSidebar(false);
    }
    syncTerminalLayout();
  });
  if (window.visualViewport) {
    window.visualViewport.addEventListener("resize", handleMobileViewportChange);
  }
  approvalDetails.addEventListener("toggle", () => {
    localStorage.setItem("approval_collapsed", approvalDetails.open ? "0" : "1");
    setTimeout(syncTerminalLayout, 0);
  });

  term.onData((data) => {
    console.log("[xterm] onData len=%d session=%s", data.length, state.selectedSessionID);
    sendWS({
      type: "term_in",
      session_id: state.selectedSessionID,
      data_b64: bytesToB64(data),
    });
  });

  saveTokenBtn.addEventListener("click", () => {
    applyUIToken(tokenInput.value.trim());
  });

  adminTypeSelect.addEventListener("change", syncAdminRoleState);
  adminGenerateBtn.addEventListener("click", createAdminToken);
  adminListTokensBtn.addEventListener("click", () => listAdminTokens());
  adminCopyTokenBtn.addEventListener("click", copyGeneratedToken);
  adminUseUiTokenBtn.addEventListener("click", () => {
    const generated = state.lastGeneratedToken;
    if (!generated || generated.type !== "ui" || !generated.token) {
      return;
    }
    applyUIToken(generated.token);
    setAdminMessage("Generated UI token is now active.");
  });
  syncAdminRoleState();
  renderAdminResult(null);
  renderAdminTokens();

  document.getElementById("refreshServersBtn").addEventListener("click", fetchServers);
  document.getElementById("refreshSessionsBtn").addEventListener("click", fetchSessions);

  document.getElementById("newSessionBtn").addEventListener("click", async () => {
    if (!state.selectedServerID) {
      alert("select a server first");
      return;
    }
    const cwd = cwdInput.value.trim();
    if (!cwd) {
      alert("cwd is required");
      return;
    }
    const resumeID = resumeInput.value.trim();
    const env = parseEnv(envInput.value);
    const body = {
      server_id: state.selectedServerID,
      cwd,
      env,
      cols: term.cols,
      rows: term.rows,
    };
    if (resumeID) {
      body.resume_id = resumeID;
    }
    const resp = await api("/api/sessions", {
      method: "POST",
      body: JSON.stringify(body),
    });
    if (!resp.ok) {
      alert(await resp.text());
      return;
    }
    const session = await resp.json();
    await fetchSessions();
    attachSession(session.session_id);
  });

  bindScrollButton("scrollUp", -1);
  bindScrollButton("scrollDown", 1);
  document.getElementById("keyTab").addEventListener("click", () => sendQuickKey("\t"));
  document.getElementById("keyEsc").addEventListener("click", () => sendQuickKey("\u001b"));
  document.getElementById("keyCtrlC").addEventListener("click", () => sendQuickKey("\u0003"));
  document.getElementById("keyUp").addEventListener("click", () => sendQuickKey("\x1b[A"));
  document.getElementById("keyDown").addEventListener("click", () => sendQuickKey("\x1b[B"));
  document.getElementById("keyRight").addEventListener("click", () => sendQuickKey("\x1b[C"));
  document.getElementById("keyLeft").addEventListener("click", () => sendQuickKey("\x1b[D"));
  document.getElementById("keyEnter").addEventListener("click", () => sendQuickKey("\r"));

  function bindScrollButton(buttonID, pageDelta) {
    const button = document.getElementById(buttonID);
    const step = () => term.scrollPages(pageDelta);
    let repeatDelayTimer = 0;
    let repeatTimer = 0;
    let suppressClickUntil = 0;

    const clearTimers = () => {
      if (repeatDelayTimer) {
        window.clearTimeout(repeatDelayTimer);
        repeatDelayTimer = 0;
      }
      if (repeatTimer) {
        window.clearInterval(repeatTimer);
        repeatTimer = 0;
      }
    };

    button.addEventListener("pointerdown", (event) => {
      if (event.button !== 0) {
        return;
      }
      event.preventDefault();
      if (button.setPointerCapture) {
        button.setPointerCapture(event.pointerId);
      }
      step();
      suppressClickUntil = Date.now() + 700;
      clearTimers();
      repeatDelayTimer = window.setTimeout(() => {
        repeatTimer = window.setInterval(step, 110);
      }, 300);
    });

    const stopRepeat = () => clearTimers();
    button.addEventListener("pointerup", stopRepeat);
    button.addEventListener("pointercancel", stopRepeat);
    button.addEventListener("lostpointercapture", stopRepeat);

    // Keep keyboard accessibility: Enter/Space still triggers a single scroll.
    button.addEventListener("click", () => {
      if (Date.now() < suppressClickUntil) {
        return;
      }
      step();
    });
  }

  async function refreshAll() {
    await fetchServers();
    await fetchSessions();
  }

  async function fetchServers() {
    const resp = await api("/api/servers");
    if (!resp.ok) {
      return;
    }
    const body = await resp.json();
    state.servers = body.servers || [];
    if (!state.selectedServerID && state.servers.length) {
      state.selectedServerID = state.servers[0].server_id;
    }
    renderServers();
  }

  async function fetchSessions() {
    const q = state.selectedServerID ? `?server_id=${encodeURIComponent(state.selectedServerID)}` : "";
    const resp = await api(`/api/sessions${q}`);
    if (!resp.ok) {
      return;
    }
    const body = await resp.json();
    state.sessions = body.sessions || [];
    renderSessions();
  }

  function renderEmptyItem(list, text) {
    const li = document.createElement("li");
    li.className = "list-empty";
    li.textContent = text;
    list.appendChild(li);
  }

  function renderServers() {
    serversList.innerHTML = "";
    if (!state.servers.length) {
      renderEmptyItem(serversList, "No servers");
      return;
    }
    for (const s of state.servers) {
      const li = document.createElement("li");
      li.classList.add("server-item");
      if (s.server_id === state.selectedServerID) li.classList.add("selected");
      const statusClass = s.status === "online" ? "badge-online" : "badge-offline";
      const tags = (s.tags || []).map(escapeHtml).join(", ");
      li.innerHTML = `
        <div class="server-main">
          <strong class="server-id">${escapeHtml(s.server_id)}</strong>
          <span class="badge ${statusClass}">${escapeHtml(s.status)}</span>
        </div>
        <div class="server-sub">
          <span class="server-host">${escapeHtml(s.hostname || "-")}</span>
          ${tags ? `<span class="server-tags">${tags}</span>` : ""}
        </div>
      `;
      li.addEventListener("click", async () => {
        state.selectedServerID = s.server_id;
        renderServers();
        await fetchSessions();
        closeSidebarOnMobile();
      });
      serversList.appendChild(li);
    }
  }

  function renderSessions() {
    sessionsList.innerHTML = "";
    if (!state.sessions.length) {
      renderEmptyItem(sessionsList, "No sessions");
      return;
    }
    for (const s of state.sessions) {
      const li = document.createElement("li");
      li.classList.add("session-item");
      if (s.session_id === state.selectedSessionID) li.classList.add("selected");
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
        ${
          s.resume_id || s.exit_reason
            ? `<div class="session-detail">
                ${s.resume_id ? `<span>resume ${escapeHtml(s.resume_id)}</span>` : ""}
                ${s.exit_reason ? `<span>reason ${escapeHtml(s.exit_reason)}</span>` : ""}
              </div>`
            : ""
        }
        <div class="session-actions">
          ${s.resume_id ? `<button type="button" data-action="resume" class="btn-secondary">Resume</button>` : ""}
          <button type="button" data-action="delete" class="btn-danger" ${canDelete ? "" : "disabled"}>Delete</button>
        </div>
      `;
      if (s.resume_id) {
        const resumeBtn = li.querySelector('[data-action="resume"]');
        resumeBtn.addEventListener("click", async (e) => {
          e.stopPropagation();
          await resumeSession(s);
        });
      }
      const deleteBtn = li.querySelector('[data-action="delete"]');
      deleteBtn.addEventListener("click", async (e) => {
        e.stopPropagation();
        await deleteSession(s);
      });
      li.addEventListener("click", () => attachSession(s.session_id));
      sessionsList.appendChild(li);
    }
  }

  async function deleteSession(session) {
    const sessionID = (session && session.session_id) || "";
    if (!sessionID) {
      return;
    }
    if (session.status === "running") {
      alert("cannot delete running session");
      return;
    }
    if (!window.confirm(`Delete session ${sessionID.slice(0, 8)}?`)) {
      return;
    }
    const resp = await api(`/api/sessions/${encodeURIComponent(sessionID)}`, {
      method: "DELETE",
    });
    if (!resp.ok) {
      alert(await resp.text());
      return;
    }
    if (state.selectedSessionID === sessionID) {
      state.selectedSessionID = "";
      state.pendingFirstOutputSessionID = "";
      currentSessionLabel.textContent = "Session: (none)";
      term.reset();
      term.scrollToBottom();
    }
    for (const [eventID, approval] of state.approvals.entries()) {
      if (approval.session_id === sessionID) {
        state.approvals.delete(eventID);
      }
    }
    renderApprovals();
    await fetchSessions();
  }

  async function resumeSession(source) {
    const resumeID = (source.resume_id || "").trim();
    if (!resumeID) {
      alert("resume id is required");
      return;
    }
    const serverID = source.server_id || state.selectedServerID;
    if (!serverID) {
      alert("select a server first");
      return;
    }
    const cwd = (source.cwd || "").trim();
    if (!cwd) {
      alert("cwd is required");
      return;
    }
    const body = {
      server_id: serverID,
      cwd,
      resume_id: resumeID,
      env: parseEnv(envInput.value),
      cols: term.cols,
      rows: term.rows,
    };
    const resp = await api("/api/sessions", {
      method: "POST",
      body: JSON.stringify(body),
    });
    if (!resp.ok) {
      alert(await resp.text());
      return;
    }
    const session = await resp.json();
    state.selectedServerID = serverID;
    await fetchSessions();
    attachSession(session.session_id);
  }

  function renderApprovals() {
    approvalList.innerHTML = "";
    const values = Array.from(state.approvals.values()).sort((a, b) => b.ts_ms - a.ts_ms);
    let pendingCount = 0;
    for (const ev of values) {
      if (ev.resolved) {
        continue;
      }
      pendingCount++;
      const li = document.createElement("li");
      li.innerHTML = `
        <div><strong>${escapeHtml(ev.session_id.slice(0, 8))}</strong> @ ${escapeHtml(ev.server_id)}</div>
      `;
      li.classList.add("approval-item");
      li.tabIndex = 0;
      li.setAttribute("role", "button");
      li.setAttribute("aria-label", `Open approval for session ${ev.session_id.slice(0, 8)} on ${ev.server_id}`);
      const openApproval = () => attachSession(ev.session_id);
      li.addEventListener("click", openApproval);
      li.addEventListener("keydown", (e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          openApproval();
        }
      });
      approvalList.appendChild(li);
    }
    approvalCount.textContent = String(pendingCount);
  }

  const wsStatusEl = document.getElementById("wsStatus");
  function setWSStatus(connected) {
    if (connected) {
      wsStatusEl.textContent = "WS: connected";
      wsStatusEl.className = "badge badge-online";
    } else {
      wsStatusEl.textContent = "WS: disconnected";
      wsStatusEl.className = "badge badge-offline";
    }
  }

  function connectWS() {
    const scheme = window.location.protocol === "https:" ? "wss" : "ws";
    const url = `${scheme}://${window.location.host}/ws/client?token=${encodeURIComponent(state.token)}`;
    console.log("[ws] connecting", url);
    state.ws = new WebSocket(url);
    state.ws.onopen = () => {
      console.log("[ws] connected");
      setWSStatus(true);
      if (state.selectedSessionID) {
        sendWS({
          type: "attach",
          data: { session_id: state.selectedSessionID, since_seq: 0 },
        });
      }
      sendResize();
    };
    state.ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        handleWS(msg);
      } catch (e) {
        console.error("[ws] parse error", e);
      }
    };
    state.ws.onerror = (e) => {
      console.error("[ws] error", e);
      setWSStatus(false);
    };
    state.ws.onclose = (e) => {
      console.log("[ws] closed code=%d reason=%s", e.code, e.reason);
      setWSStatus(false);
      setTimeout(reconnectWS, 1000);
    };
  }

  function reconnectWS() {
    if (state.ws) {
      state.ws.onclose = null;
      state.ws.close();
    }
    connectWS();
  }

  function handleWS(msg) {
    if (msg.type === "term_out") {
      if (msg.session_id === state.selectedSessionID && msg.data_b64) {
        if (state.pendingFirstOutputSessionID === msg.session_id) {
          term.write(b64ToBytes(msg.data_b64), () => {
            term.scrollToBottom();
            state.pendingFirstOutputSessionID = "";
            currentSessionLabel.textContent = `Session: ${msg.session_id}`;
          });
        } else {
          term.write(b64ToBytes(msg.data_b64));
        }
      }
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
    if (msg.type === "error" && msg.data) {
      console.error("[ws] server error", msg.session_id || "", msg.data);
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

  function attachSession(sessionID) {
    if (!sessionID) {
      return;
    }
    state.pendingFirstOutputSessionID = sessionID;
    state.selectedSessionID = sessionID;
    currentSessionLabel.textContent = `Session: ${sessionID} (loading...)`;
    renderSessions();
    term.reset();
    term.scrollToBottom();
    sendWS({
      type: "attach",
      data: { session_id: sessionID, since_seq: 0 },
    });
    sendResize();
    closeSidebarOnMobile();
  }

  function sendQuickKey(keyValue) {
    if (!state.selectedSessionID) {
      alert("No session attached — click a session first");
      return;
    }
    if (!state.ws || state.ws.readyState !== WebSocket.OPEN) {
      alert("WebSocket not connected");
      return;
    }
    sendWS({
      type: "term_in",
      session_id: state.selectedSessionID,
      data_b64: bytesToB64(keyValue),
    });
  }

  function sendResize() {
    if (!state.selectedSessionID) {
      return;
    }
    sendWS({
      type: "resize",
      session_id: state.selectedSessionID,
      data: { cols: term.cols, rows: term.rows },
    });
  }

  function sendWS(msg) {
    if (!state.ws || state.ws.readyState !== WebSocket.OPEN) {
      console.warn("[ws] not connected, dropping msg type=%s", msg.type);
      return;
    }
    const raw = JSON.stringify(msg);
    console.log("[ws] >>", msg.type, "session=", msg.session_id || "(none)");
    state.ws.send(raw);
  }

  function applyUIToken(token) {
    state.token = token || "";
    tokenInput.value = state.token;
    localStorage.setItem("ui_token", state.token);
    reconnectWS();
    refreshAll();
  }

  function syncAdminRoleState() {
    const isUI = adminTypeSelect.value === "ui";
    adminRoleSelect.disabled = !isUI;
  }

  function setAdminMessage(message, isError = false) {
    adminMessage.textContent = message || "";
    adminMessage.classList.toggle("error", isError);
  }

  function normalizeValue(value, fallback = "-") {
    if (value === undefined || value === null || value === "") {
      return fallback;
    }
    return String(value);
  }

  function makeAdminFieldRow(label, value, { mono = false } = {}) {
    const row = document.createElement("div");
    row.className = "admin-field-row";
    const key = document.createElement("div");
    key.className = "admin-field-key";
    key.textContent = label;
    const val = document.createElement("div");
    val.className = `admin-field-value${mono ? " mono" : ""}`;
    val.textContent = normalizeValue(value);
    row.appendChild(key);
    row.appendChild(val);
    return row;
  }

  function renderAdminResult(data) {
    adminResult.innerHTML = "";
    if (!data) {
      const empty = document.createElement("div");
      empty.className = "admin-result-empty";
      empty.textContent = "(no token generated)";
      adminResult.appendChild(empty);
      return;
    }
    const card = document.createElement("div");
    card.className = "admin-result-card";

    const header = document.createElement("div");
    header.className = "admin-result-header";
    const title = document.createElement("strong");
    title.textContent = "Latest Generated Token";
    const typeBadge = document.createElement("span");
    typeBadge.className = "badge";
    typeBadge.textContent = normalizeValue(data.type);
    header.appendChild(title);
    header.appendChild(typeBadge);

    const fields = document.createElement("div");
    fields.className = "admin-field-list";
    fields.appendChild(makeAdminFieldRow("token", data.token, { mono: true }));
    fields.appendChild(makeAdminFieldRow("token_id", data.token_id, { mono: true }));
    fields.appendChild(makeAdminFieldRow("tenant", data.tenant_id, { mono: true }));
    fields.appendChild(makeAdminFieldRow("role", data.role));
    fields.appendChild(makeAdminFieldRow("name", data.name));
    fields.appendChild(makeAdminFieldRow("created", formatTime(data.created_at_ms)));

    card.appendChild(header);
    card.appendChild(fields);
    adminResult.appendChild(card);
  }

  function renderAdminTokens() {
    adminTokensList.innerHTML = "";
    if (!state.adminTokens.length) {
      const li = document.createElement("li");
      li.className = "admin-token-item";
      li.textContent = "(no tokens)";
      adminTokensList.appendChild(li);
      return;
    }
    for (const rec of state.adminTokens) {
      const li = document.createElement("li");
      li.className = "admin-token-item";

      const head = document.createElement("div");
      head.className = "admin-token-head";
      const id = document.createElement("strong");
      id.textContent = rec.token_id ? rec.token_id.slice(0, 8) : "(unknown)";
      const status = document.createElement("span");
      status.className = rec.revoked ? "badge badge-offline" : "badge badge-online";
      status.textContent = rec.revoked ? "revoked" : "active";
      head.appendChild(id);
      head.appendChild(status);

      const fields = document.createElement("div");
      fields.className = "admin-field-list";
      fields.appendChild(makeAdminFieldRow("type", rec.type));
      fields.appendChild(makeAdminFieldRow("role", rec.role));
      fields.appendChild(makeAdminFieldRow("tenant", rec.tenant_id, { mono: true }));
      fields.appendChild(makeAdminFieldRow("token_id", rec.token_id, { mono: true }));
      fields.appendChild(makeAdminFieldRow("created", formatTime(rec.created_at_ms)));
      fields.appendChild(makeAdminFieldRow("name", rec.name));

      li.appendChild(head);
      li.appendChild(fields);

      if (!rec.revoked && rec.token_id) {
        const actions = document.createElement("div");
        actions.className = "admin-token-actions";
        const revokeBtn = document.createElement("button");
        revokeBtn.type = "button";
        revokeBtn.className = "btn-danger";
        revokeBtn.textContent = "Revoke";
        revokeBtn.addEventListener("click", () => revokeAdminToken(rec.token_id));
        actions.appendChild(revokeBtn);
        li.appendChild(actions);
      }
      adminTokensList.appendChild(li);
    }
  }

  function formatTime(ts) {
    const n = Number(ts);
    if (!Number.isFinite(n) || n <= 0) {
      return String(ts || "-");
    }
    try {
      return new Date(n).toLocaleString();
    } catch (_err) {
      return String(ts);
    }
  }

  async function createAdminToken() {
    const token = adminTokenInput.value.trim();
    if (!token) {
      setAdminMessage("Admin token is required.", true);
      return;
    }
    const payload = {
      type: adminTypeSelect.value,
    };
    if (payload.type === "ui") {
      payload.role = adminRoleSelect.value;
    }
    const tenantID = adminTenantInput.value.trim();
    if (tenantID) {
      payload.tenant_id = tenantID;
    }
    const name = adminNameInput.value.trim();
    if (name) {
      payload.name = name;
    }
    state.adminToken = token;
    localStorage.setItem("admin_token", state.adminToken);
    setAdminMessage("Generating token...");
    let resp;
    try {
      resp = await adminApi("/admin/tokens", {
        method: "POST",
        body: JSON.stringify(payload),
      });
    } catch (err) {
      setAdminMessage(`Request failed: ${String(err)}`, true);
      return;
    }
    if (!resp.ok) {
      setAdminMessage(await resp.text(), true);
      return;
    }
    const result = await resp.json();
    state.lastGeneratedToken = result;
    if (result.tenant_id) {
      adminTenantInput.value = result.tenant_id;
    }
    adminCopyTokenBtn.disabled = !result.token;
    adminUseUiTokenBtn.disabled = !(result.type === "ui" && result.token);
    renderAdminResult(result);
    setAdminMessage("Token generated.");
  }

  async function listAdminTokens(showLoading = true) {
    const token = adminTokenInput.value.trim();
    if (!token) {
      setAdminMessage("Admin token is required.", true);
      return;
    }
    state.adminToken = token;
    localStorage.setItem("admin_token", state.adminToken);
    const tenantID = adminTenantInput.value.trim();
    const path = tenantID ? `/admin/tokens?tenant_id=${encodeURIComponent(tenantID)}` : "/admin/tokens";
    if (showLoading) {
      setAdminMessage("Loading tokens...");
    }
    let resp;
    try {
      resp = await adminApi(path);
    } catch (err) {
      setAdminMessage(`Request failed: ${String(err)}`, true);
      return;
    }
    if (!resp.ok) {
      setAdminMessage(await resp.text(), true);
      return;
    }
    const body = await resp.json();
    const tokens = Array.isArray(body.tokens) ? body.tokens : [];
    tokens.sort((a, b) => Number(b.created_at_ms || 0) - Number(a.created_at_ms || 0));
    state.adminTokens = tokens;
    renderAdminTokens();
    setAdminMessage(`Loaded ${tokens.length} token(s).`);
  }

  async function copyGeneratedToken() {
    const token = state.lastGeneratedToken && state.lastGeneratedToken.token;
    if (!token) {
      return;
    }
    if (!navigator.clipboard || !navigator.clipboard.writeText) {
      setAdminMessage("Clipboard not available. Copy token from the result box.", true);
      return;
    }
    try {
      await navigator.clipboard.writeText(token);
      setAdminMessage("Token copied to clipboard.");
    } catch (_err) {
      setAdminMessage("Copy failed. Copy token from the result box.", true);
    }
  }

  async function revokeAdminToken(tokenID) {
    if (!tokenID) {
      return;
    }
    if (!window.confirm(`Revoke token ${tokenID.slice(0, 8)}?`)) {
      return;
    }
    const token = adminTokenInput.value.trim();
    if (!token) {
      setAdminMessage("Admin token is required.", true);
      return;
    }
    state.adminToken = token;
    localStorage.setItem("admin_token", state.adminToken);
    setAdminMessage("Revoking token...");
    let resp;
    try {
      resp = await adminApi(`/admin/tokens/${encodeURIComponent(tokenID)}/revoke`, {
        method: "POST",
      });
    } catch (err) {
      setAdminMessage(`Request failed: ${String(err)}`, true);
      return;
    }
    if (!resp.ok) {
      setAdminMessage(await resp.text(), true);
      return;
    }
    for (const rec of state.adminTokens) {
      if (rec.token_id === tokenID) {
        rec.revoked = true;
      }
    }
    renderAdminTokens();
    setAdminMessage("Token revoked.");
  }

  function api(path, init = {}) {
    const headers = new Headers(init.headers || {});
    headers.set("Authorization", `Bearer ${state.token}`);
    if (init.body && !headers.has("Content-Type")) {
      headers.set("Content-Type", "application/json");
    }
    return fetch(path, { ...init, headers });
  }

  function adminApi(path, init = {}) {
    const headers = new Headers(init.headers || {});
    headers.set("Authorization", `Bearer ${state.adminToken}`);
    if (init.body && !headers.has("Content-Type")) {
      headers.set("Content-Type", "application/json");
    }
    return fetch(path, { ...init, headers });
  }

  function parseEnv(input) {
    const env = {};
    const pairs = (input || "").split(",");
    for (const pair of pairs) {
      const trimmed = pair.trim();
      if (!trimmed) {
        continue;
      }
      const idx = trimmed.indexOf("=");
      if (idx <= 0) {
        continue;
      }
      env[trimmed.slice(0, idx)] = trimmed.slice(idx + 1);
    }
    return env;
  }

  function escapeHtml(str) {
    return String(str)
      .replaceAll("&", "&amp;")
      .replaceAll("<", "&lt;")
      .replaceAll(">", "&gt;")
      .replaceAll('"', "&quot;")
      .replaceAll("'", "&#039;");
  }

  connectWS();
  refreshAll();
})();
