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

  const adminTokenCacheKey = "admin_token_cache";

  function loadAdminTokenCache() {
    try {
      const raw = localStorage.getItem(adminTokenCacheKey);
      if (!raw) {
        return new Map();
      }
      const parsed = JSON.parse(raw);
      if (!parsed || typeof parsed !== "object") {
        return new Map();
      }
      const map = new Map();
      for (const [tokenID, token] of Object.entries(parsed)) {
        if (tokenID && typeof token === "string") {
          map.set(tokenID, token);
        }
      }
      return map;
    } catch (_err) {
      return new Map();
    }
  }

  const state = {
    token: localStorage.getItem("ui_token") || "admin-dev-token",
    adminToken: localStorage.getItem("admin_token") || "admin-dev-token",
    tenantToken: localStorage.getItem("tenant_token") || "",
    tenantID: localStorage.getItem("tenant_id") || "",
    adminVerified: false,
    tenantVerified: false,
    adminVerifyInFlight: false,
    tenantVerifyInFlight: false,
    selectedServerID: "",
    selectedSessionID: "",
    pendingFirstOutputSessionID: "",
    ws: null,
    approvals: new Map(),
    sessions: [],
    servers: [],
    lastGeneratedToken: null,
    lastTenantTokens: null,
    adminTenantTokens: [],
    selectedAdminTokenID: "",
    adminTokenSecrets: loadAdminTokenCache(),
    adminServers: [],
    adminSessions: [],
    adminActiveTab: "overview",
  };

  const tokenInput = document.getElementById("tokenInput");
  const saveTokenBtn = document.getElementById("saveTokenBtn");
  const adminTokenInput = document.getElementById("adminTokenInput");
  const adminGenerateBtn = document.getElementById("adminGenerateBtn");
  const adminCopyTokenBtn = document.getElementById("adminCopyTokenBtn");
  const adminListTenantsBtn = document.getElementById("adminListTenantsBtn");
  const adminExportBtn = document.getElementById("adminExportBtn");
  const adminExportCsvBtn = document.getElementById("adminExportCsvBtn");
  const adminMessage = document.getElementById("adminMessage");
  const adminResult = document.getElementById("adminResult");
  const adminTenantList = document.getElementById("adminTenantList");
  const adminVerifyBtn = document.getElementById("adminVerifyBtn");
  const adminGateMessage = document.getElementById("adminGateMessage");
  const adminContent = document.getElementById("adminContent");
  const adminServersSearch = document.getElementById("adminServersSearch");
  const adminSessionsSearch = document.getElementById("adminSessionsSearch");
  const adminSessionsStatusFilter = document.getElementById("adminSessionsStatusFilter");
  const adminTokenSearch = document.getElementById("adminTokenSearch");
  const tenantTokenInput = document.getElementById("tenantTokenInput");
  const tenantRoleSelect = document.getElementById("tenantRoleSelect");
  const tenantTenantInput = document.getElementById("tenantTenantInput");
  const tenantGenerateBtn = document.getElementById("tenantGenerateBtn");
  const tenantCopyUiTokenBtn = document.getElementById("tenantCopyUiTokenBtn");
  const tenantCopyAgentTokenBtn = document.getElementById("tenantCopyAgentTokenBtn");
  const tenantMessage = document.getElementById("tenantMessage");
  const tenantResult = document.getElementById("tenantResult");
  const tenantVerifyBtn = document.getElementById("tenantVerifyBtn");
  const tenantGateMessage = document.getElementById("tenantGateMessage");
  const tenantContent = document.getElementById("tenantContent");
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
  const mobileMedia = window.matchMedia("(max-width: 900px)");
  let mobileKeyboardOpen = false;

  const isControllerPage = Boolean(document.getElementById("terminal"));
  const isAdminPage = Boolean(adminVerifyBtn);
  const isTenantPage = Boolean(tenantVerifyBtn);

  if (tokenInput) {
    tokenInput.value = state.token;
  }
  if (adminTokenInput) {
    adminTokenInput.value = state.adminToken;
  }
  if (tenantTokenInput) {
    tenantTokenInput.value = state.tenantToken;
  }
  if (tenantTenantInput) {
    tenantTenantInput.value = state.tenantID;
  }

  let term = null;
  let fitAddon = null;
  if (isControllerPage) {
    term = new Terminal({
      cursorBlink: true,
      convertEol: true,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      fontSize: 14,
      lineHeight: 1.2,
      theme: { background: "#0b1020" },
    });
    fitAddon = new FitAddon.FitAddon();
    term.loadAddon(fitAddon);
    term.open(document.getElementById("terminal"));
    fitAddon.fit();
    window.addEventListener("resize", () => {
      fitAddon.fit();
      sendResize();
    });
    new ResizeObserver(() => {
      fitAddon.fit();
      sendResize();
    }).observe(document.getElementById("terminal"));
  }

  function isMobileViewport() {
    return mobileMedia.matches;
  }

  function syncTerminalLayout() {
    if (!fitAddon) {
      return;
    }
    fitAddon.fit();
    sendResize();
  }

  function handleMobileViewportChange() {
    if (!isControllerPage) {
      return;
    }
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
    if (!sidebarToggleBtn || !sidebarBackdrop) {
      return;
    }
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

  function initializeApprovalDetails() {
    if (!approvalDetails) {
      return;
    }
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

  if (isControllerPage) {
    initializeApprovalDetails();
    toggleSidebar(false);
    sidebarToggleBtn.addEventListener("click", () => toggleSidebar());
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
  }

  if (isAdminPage) {
    setAdminVerified(false, "Enter admin token to continue.");
    if (adminTokenInput.value.trim()) {
      verifyAdminToken();
    }
  }

  if (isTenantPage) {
    setTenantVerified(false, "Enter tenant id + token to continue.");
    if (tenantTokenInput.value.trim() && tenantTenantInput.value.trim()) {
      verifyTenantToken();
    }
  }

  if (isControllerPage) {
    saveTokenBtn.addEventListener("click", () => {
      applyUIToken(tokenInput.value.trim());
    });

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
  }

  if (isAdminPage) {
    adminVerifyBtn.addEventListener("click", verifyAdminToken);
    adminTokenInput.addEventListener("input", () => {
      if (state.adminVerified) {
        setAdminVerified(false, "Token changed. Please verify again.");
      } else {
        setGateMessage(adminGateMessage, "");
      }
    });
    adminGenerateBtn.addEventListener("click", createAdminToken);
    adminCopyTokenBtn.addEventListener("click", copyGeneratedToken);
    adminListTenantsBtn.addEventListener("click", () => {
      listAdminTenantTokens();
    });
    adminExportBtn.addEventListener("click", exportAdminTokens);
    adminExportCsvBtn.addEventListener("click", exportAdminTokensCsv);
    renderAdminResult(null);
    renderAdminTenantTokens();
    updateAdminCopyState();

    // Tab switching
    for (const tab of document.querySelectorAll(".admin-tab")) {
      tab.addEventListener("click", () => switchAdminTab(tab.dataset.tab));
    }

    // Refresh buttons
    const adminRefreshOverviewBtn = document.getElementById("adminRefreshOverviewBtn");
    const adminRefreshServersBtn = document.getElementById("adminRefreshServersBtn");
    const adminRefreshSessionsBtn = document.getElementById("adminRefreshSessionsBtn");
    if (adminRefreshOverviewBtn) adminRefreshOverviewBtn.addEventListener("click", refreshAdminOverview);
    if (adminRefreshServersBtn) adminRefreshServersBtn.addEventListener("click", fetchAdminServers);
    if (adminRefreshSessionsBtn) adminRefreshSessionsBtn.addEventListener("click", fetchAdminSessions);

    // Search / filter
    if (adminServersSearch) adminServersSearch.addEventListener("input", debounce(renderAdminServers, 200));
    if (adminSessionsSearch) adminSessionsSearch.addEventListener("input", debounce(renderAdminSessions, 200));
    if (adminSessionsStatusFilter) adminSessionsStatusFilter.addEventListener("change", renderAdminSessions);
    if (adminTokenSearch) adminTokenSearch.addEventListener("input", debounce(renderAdminTenantTokens, 200));
  }

  if (isTenantPage) {
    tenantVerifyBtn.addEventListener("click", verifyTenantToken);
    tenantTokenInput.addEventListener("input", () => {
      if (state.tenantVerified) {
        setTenantVerified(false, "Token changed. Please verify again.");
      } else {
        setGateMessage(tenantGateMessage, "");
      }
    });
    tenantTenantInput.addEventListener("input", () => {
      if (state.tenantVerified) {
        setTenantVerified(false, "Tenant changed. Please verify again.");
      } else {
        setGateMessage(tenantGateMessage, "");
      }
    });
    tenantGenerateBtn.addEventListener("click", createTenantTokens);
    tenantCopyUiTokenBtn.addEventListener("click", () => copyTenantToken("ui"));
    tenantCopyAgentTokenBtn.addEventListener("click", () => copyTenantToken("agent"));
    renderTenantResult(null);
  }

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
    if (!approvalList || !approvalCount) {
      return;
    }
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
    if (!wsStatusEl) {
      return;
    }
    if (connected) {
      wsStatusEl.textContent = "WS: connected";
      wsStatusEl.className = "badge badge-online";
    } else {
      wsStatusEl.textContent = "WS: disconnected";
      wsStatusEl.className = "badge badge-offline";
    }
  }

  function connectWS() {
    if (!isControllerPage) {
      return;
    }
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
    if (!isControllerPage) {
      return;
    }
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
    if (!state.selectedSessionID || !term) {
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
    if (tokenInput) {
      tokenInput.value = state.token;
    }
    localStorage.setItem("ui_token", state.token);
    if (isControllerPage) {
      reconnectWS();
      refreshAll();
    }
  }

  function setAdminMessage(message, isError = false) {
    adminMessage.textContent = message || "";
    adminMessage.classList.toggle("error", isError);
  }

  function setTenantMessage(message, isError = false) {
    tenantMessage.textContent = message || "";
    tenantMessage.classList.toggle("error", isError);
  }

  function setGateMessage(el, message, isError = false) {
    if (!el) {
      return;
    }
    el.textContent = message || "";
    el.classList.toggle("error", isError);
  }

  function setAdminVerified(verified, message = "", isError = false) {
    state.adminVerified = verified;
    adminContent.hidden = !verified;
    adminVerifyBtn.textContent = verified ? "Re-Verify Token" : "Verify Token";
    setGateMessage(adminGateMessage, message, isError);
    if (adminListTenantsBtn) {
      adminListTenantsBtn.disabled = !verified;
    }
    if (adminExportBtn) {
      adminExportBtn.disabled = !verified;
    }
    if (adminExportCsvBtn) {
      adminExportCsvBtn.disabled = !verified;
    }
    if (!verified) {
      setAdminMessage("");
      adminCopyTokenBtn.disabled = true;
      state.lastGeneratedToken = null;
      state.adminTenantTokens = [];
      state.adminServers = [];
      state.adminSessions = [];
      state.selectedAdminTokenID = "";
      if (adminTenantList) {
        adminTenantList.hidden = true;
      }
      renderAdminResult(null);
      renderAdminTenantTokens();
      updateAdminCopyState();
    } else {
      refreshAdminOverview();
    }
  }

  function setTenantVerified(verified, message = "", isError = false) {
    state.tenantVerified = verified;
    tenantContent.hidden = !verified;
    tenantVerifyBtn.textContent = verified ? "Re-Verify Token" : "Verify Token";
    setGateMessage(tenantGateMessage, message, isError);
    if (!verified) {
      setTenantMessage("");
      tenantCopyUiTokenBtn.disabled = true;
      tenantCopyAgentTokenBtn.disabled = true;
      state.lastTenantTokens = null;
      renderTenantResult(null);
    }
  }

  function persistAdminTokenCache() {
    try {
      const payload = {};
      for (const [tokenID, token] of state.adminTokenSecrets) {
        payload[tokenID] = token;
      }
      localStorage.setItem(adminTokenCacheKey, JSON.stringify(payload));
    } catch (_err) {
      // ignore storage errors
    }
  }

  function cacheAdminToken(tokenID, token) {
    if (!tokenID || !token) {
      return;
    }
    state.adminTokenSecrets.set(tokenID, token);
    persistAdminTokenCache();
  }

  function getCachedAdminToken(tokenID) {
    if (!tokenID) {
      return "";
    }
    return state.adminTokenSecrets.get(tokenID) || "";
  }

  function updateAdminCopyState() {
    if (!adminCopyTokenBtn) {
      return;
    }
    const selectedToken = getCachedAdminToken(state.selectedAdminTokenID);
    const latestToken = state.lastGeneratedToken && state.lastGeneratedToken.token;
    if (state.selectedAdminTokenID) {
      adminCopyTokenBtn.disabled = !selectedToken;
      return;
    }
    adminCopyTokenBtn.disabled = !latestToken;
  }

  function formatExportFilename(prefix) {
    const now = new Date();
    const pad = (n) => String(n).padStart(2, "0");
    const stamp = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`;
    return `${prefix}-${stamp}.json`;
  }

  function formatExportCsvFilename(prefix) {
    const now = new Date();
    const pad = (n) => String(n).padStart(2, "0");
    const stamp = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`;
    return `${prefix}-${stamp}.csv`;
  }

  function toCsvValue(value) {
    const raw = value === undefined || value === null ? "" : String(value);
    const escaped = raw.replaceAll('"', '""');
    if (/[",\n]/.test(escaped)) {
      return `"${escaped}"`;
    }
    return escaped;
  }

  function buildAdminExportRecords() {
    const records = [];
    const seen = new Set();
    for (const rec of state.adminTenantTokens) {
      if (!rec || !rec.token_id || seen.has(rec.token_id)) {
        continue;
      }
      seen.add(rec.token_id);
      records.push({
        tenant_id: rec.tenant_id || "",
        token_id: rec.token_id,
        token: getCachedAdminToken(rec.token_id) || "",
        created_at_ms: rec.created_at_ms || 0,
        revoked: Boolean(rec.revoked),
      });
    }
    if (records.length === 0 && state.lastGeneratedToken && state.lastGeneratedToken.token_id) {
      records.push({
        tenant_id: state.lastGeneratedToken.tenant_id || "",
        token_id: state.lastGeneratedToken.token_id,
        token: state.lastGeneratedToken.token || "",
        created_at_ms: state.lastGeneratedToken.created_at_ms || 0,
        revoked: false,
      });
    }
    return records;
  }

  function exportAdminTokensCsv() {
    if (!state.adminVerified) {
      setAdminMessage("Verify admin token first.", true);
      return;
    }
    const records = buildAdminExportRecords();
    if (!records.length) {
      setAdminMessage("No tenant tokens available to export.", true);
      return;
    }
    const header = ["tenant_id", "token_id", "token", "created_at_ms", "revoked"];
    const lines = [header.map(toCsvValue).join(",")];
    for (const rec of records) {
      lines.push([
        toCsvValue(rec.tenant_id),
        toCsvValue(rec.token_id),
        toCsvValue(rec.token),
        toCsvValue(rec.created_at_ms),
        toCsvValue(rec.revoked),
      ].join(","));
    }
    const csv = lines.join("\n");
    const blob = new Blob([csv], { type: "text/csv" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = formatExportCsvFilename("tenant-tokens");
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
    setAdminMessage(`Exported ${records.length} token(s) to CSV.`);
  }

  function exportAdminTokens() {
    if (!state.adminVerified) {
      setAdminMessage("Verify admin token first.", true);
      return;
    }
    const records = buildAdminExportRecords();
    if (!records.length) {
      setAdminMessage("No tenant tokens available to export.", true);
      return;
    }
    const payload = {
      exported_at_ms: Date.now(),
      tokens: records,
    };
    const blob = new Blob([JSON.stringify(payload, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = formatExportFilename("tenant-tokens");
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
    setAdminMessage(`Exported ${records.length} token(s).`);
  }

  function formatTenantTokenClipboard(tenantID, tokenLabel, tokenValue) {
    const safeTenant = tenantID || "-";
    const safeToken = tokenValue || "";
    const label = tokenLabel || "token";
    return `tenant_id: ${safeTenant}\n${label}: ${safeToken}`;
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
    title.textContent = "Latest Tenant Token";
    header.appendChild(title);

    const fields = document.createElement("div");
    fields.className = "admin-field-list";
    fields.appendChild(makeAdminFieldRow("tenant", data.tenant_id, { mono: true }));
    fields.appendChild(makeAdminFieldRow("token", data.token, { mono: true }));
    fields.appendChild(makeAdminFieldRow("token_id", data.token_id, { mono: true }));
    fields.appendChild(makeAdminFieldRow("created", formatTime(data.created_at_ms)));

    card.appendChild(header);
    card.appendChild(fields);
    adminResult.appendChild(card);
  }

  function renderAdminTenantTokens() {
    if (!adminTenantList) {
      return;
    }
    adminTenantList.innerHTML = "";
    const query = (adminTokenSearch && adminTokenSearch.value || "").toLowerCase().trim();
    const filtered = state.adminTenantTokens.filter((rec) => {
      if (!query) return true;
      return (rec.tenant_id || "").toLowerCase().includes(query)
        || (rec.token_id || "").toLowerCase().includes(query);
    });
    if (!filtered.length) {
      const li = document.createElement("li");
      li.className = "admin-token-item list-empty";
      li.textContent = "(no tenant tokens)";
      adminTenantList.appendChild(li);
      return;
    }
    for (const rec of filtered) {
      const li = document.createElement("li");
      li.className = "admin-token-item";
      const tokenID = rec.token_id || "";
      if (tokenID && tokenID === state.selectedAdminTokenID) {
        li.classList.add("selected");
      }

      const head = document.createElement("div");
      head.className = "admin-token-head";
      const id = document.createElement("strong");
      id.textContent = rec.tenant_id ? rec.tenant_id.slice(0, 8) : "(unknown)";
      const status = document.createElement("span");
      status.className = rec.revoked ? "badge badge-offline" : "badge badge-online";
      status.textContent = rec.revoked ? "revoked" : "active";
      head.appendChild(id);
      head.appendChild(status);

      const fields = document.createElement("div");
      fields.className = "admin-field-list";
      fields.appendChild(makeAdminFieldRow("tenant", rec.tenant_id, { mono: true }));
      fields.appendChild(makeAdminFieldRow("token_id", rec.token_id, { mono: true }));
      fields.appendChild(makeAdminFieldRow("created", formatTime(rec.created_at_ms)));

      li.appendChild(head);
      li.appendChild(fields);

      if (tokenID) {
        li.addEventListener("click", (event) => {
          if (event.target && event.target.closest && event.target.closest("button")) {
            return;
          }
          state.selectedAdminTokenID = state.selectedAdminTokenID === tokenID ? "" : tokenID;
          renderAdminTenantTokens();
          updateAdminCopyState();
        });
        li.addEventListener("keydown", (event) => {
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            state.selectedAdminTokenID = state.selectedAdminTokenID === tokenID ? "" : tokenID;
            renderAdminTenantTokens();
            updateAdminCopyState();
          }
        });
        li.tabIndex = 0;
      }

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
      adminTenantList.appendChild(li);
    }
  }

  function renderTenantResult(data) {
    tenantResult.innerHTML = "";
    if (!data) {
      const empty = document.createElement("div");
      empty.className = "admin-result-empty";
      empty.textContent = "(no token generated)";
      tenantResult.appendChild(empty);
      return;
    }
    const card = document.createElement("div");
    card.className = "admin-result-card";

    const header = document.createElement("div");
    header.className = "admin-result-header";
    const title = document.createElement("strong");
    title.textContent = "Latest Tenant Tokens";
    header.appendChild(title);

    const fields = document.createElement("div");
    fields.className = "admin-field-list";
    fields.appendChild(makeAdminFieldRow("tenant", data.tenant_id, { mono: true }));
    fields.appendChild(makeAdminFieldRow("revoked", data.revoked_count));
    fields.appendChild(makeAdminFieldRow("ui_token", data.ui && data.ui.token, { mono: true }));
    fields.appendChild(makeAdminFieldRow("ui_token_id", data.ui && data.ui.token_id, { mono: true }));
    fields.appendChild(makeAdminFieldRow("ui_role", data.ui && data.ui.role));
    fields.appendChild(makeAdminFieldRow("ui_created", formatTime(data.ui && data.ui.created_at_ms)));
    fields.appendChild(makeAdminFieldRow("agent_token", data.agent && data.agent.token, { mono: true }));
    fields.appendChild(makeAdminFieldRow("agent_token_id", data.agent && data.agent.token_id, { mono: true }));
    fields.appendChild(makeAdminFieldRow("agent_created", formatTime(data.agent && data.agent.created_at_ms)));

    card.appendChild(header);
    card.appendChild(fields);
    tenantResult.appendChild(card);
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

  async function verifyAdminToken() {
    const token = adminTokenInput.value.trim();
    if (!token) {
      setAdminVerified(false, "Admin token is required.", true);
      return;
    }
    if (state.adminVerifyInFlight) {
      return;
    }
    state.adminVerifyInFlight = true;
    adminVerifyBtn.disabled = true;
    adminContent.hidden = true;
    setGateMessage(adminGateMessage, "Verifying...");
    state.adminToken = token;
    localStorage.setItem("admin_token", state.adminToken);
    let resp;
    try {
      resp = await adminApi("/admin/verify");
    } catch (err) {
      setAdminVerified(false, `Request failed: ${String(err)}`, true);
      adminVerifyBtn.disabled = false;
      state.adminVerifyInFlight = false;
      return;
    }
    if (!resp.ok) {
      const msg = (await resp.text()) || "Unauthorized.";
      setAdminVerified(false, msg, true);
      adminVerifyBtn.disabled = false;
      state.adminVerifyInFlight = false;
      return;
    }
    setAdminVerified(true, "Verified.");
    adminVerifyBtn.disabled = false;
    state.adminVerifyInFlight = false;
  }

  async function verifyTenantToken() {
    const token = tenantTokenInput.value.trim();
    if (!token) {
      setTenantVerified(false, "Tenant token is required.", true);
      return;
    }
    const tenantID = tenantTenantInput.value.trim();
    if (!tenantID) {
      setTenantVerified(false, "Tenant id is required.", true);
      return;
    }
    if (state.tenantVerifyInFlight) {
      return;
    }
    state.tenantVerifyInFlight = true;
    tenantVerifyBtn.disabled = true;
    tenantContent.hidden = true;
    setGateMessage(tenantGateMessage, "Verifying...");
    state.tenantToken = token;
    localStorage.setItem("tenant_token", state.tenantToken);
    state.tenantID = tenantID;
    localStorage.setItem("tenant_id", state.tenantID);
    let resp;
    try {
      resp = await tenantApi("/tenant/verify", {
        method: "POST",
        body: JSON.stringify({ tenant_id: tenantID }),
      });
    } catch (err) {
      setTenantVerified(false, `Request failed: ${String(err)}`, true);
      tenantVerifyBtn.disabled = false;
      state.tenantVerifyInFlight = false;
      return;
    }
    if (!resp.ok) {
      const msg = (await resp.text()) || "Unauthorized.";
      setTenantVerified(false, msg, true);
      tenantVerifyBtn.disabled = false;
      state.tenantVerifyInFlight = false;
      return;
    }
    let body = {};
    try {
      body = await resp.json();
    } catch (_err) {
      body = {};
    }
    if (body && body.tenant_id && tenantTenantInput.value.trim() !== body.tenant_id) {
      tenantTenantInput.value = body.tenant_id;
    }
    setTenantVerified(true, "Verified.");
    tenantVerifyBtn.disabled = false;
    state.tenantVerifyInFlight = false;
  }

  async function createAdminToken() {
    const token = adminTokenInput.value.trim();
    if (!token) {
      setAdminMessage("Admin token is required.", true);
      return;
    }
    if (!state.adminVerified) {
      setAdminMessage("Verify admin token first.", true);
      return;
    }
    const payload = {
      type: "tenant",
    };
    state.adminToken = token;
    localStorage.setItem("admin_token", state.adminToken);
    setAdminMessage("Generating tenant token...");
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
    cacheAdminToken(result.token_id, result.token);
    state.selectedAdminTokenID = "";
    renderAdminResult(result);
    if (adminTenantList && !adminTenantList.hidden) {
      await listAdminTenantTokens(false);
    } else {
      renderAdminTenantTokens();
    }
    updateAdminCopyState();
    setAdminMessage("Token generated.");
  }

  async function copyGeneratedToken() {
    let token = "";
    let label = "Token";
    let tenantID = "";
    if (state.selectedAdminTokenID) {
      token = getCachedAdminToken(state.selectedAdminTokenID);
      if (!token) {
        setAdminMessage("Selected token not available locally.", true);
        return;
      }
      const selected = state.adminTenantTokens.find((rec) => rec.token_id === state.selectedAdminTokenID);
      tenantID = selected ? selected.tenant_id : "";
      if (!tenantID) {
        setAdminMessage("Selected tenant id not available.", true);
        return;
      }
      label = "Selected token";
    } else {
      token = state.lastGeneratedToken && state.lastGeneratedToken.token;
      if (!token) {
        return;
      }
      tenantID = state.lastGeneratedToken && state.lastGeneratedToken.tenant_id;
      if (!tenantID) {
        setAdminMessage("Tenant id not available for latest token.", true);
        return;
      }
    }
    if (!navigator.clipboard || !navigator.clipboard.writeText) {
      setAdminMessage("Clipboard not available. Copy token from the result box.", true);
      return;
    }
    try {
      await navigator.clipboard.writeText(formatTenantTokenClipboard(tenantID, "token", token));
      setAdminMessage(`${label} copied (tenant + token).`);
    } catch (_err) {
      setAdminMessage("Copy failed. Copy token from the result box.", true);
    }
  }

  async function listAdminTenantTokens(showLoading = true) {
    const token = adminTokenInput.value.trim();
    if (!token) {
      setAdminMessage("Admin token is required.", true);
      return;
    }
    if (!state.adminVerified) {
      setAdminMessage("Verify admin token first.", true);
      return;
    }
    state.adminToken = token;
    localStorage.setItem("admin_token", state.adminToken);
    if (showLoading) {
      setAdminMessage("Loading tenant tokens...");
    }
    let resp;
    try {
      resp = await adminApi("/admin/tokens");
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
    const tenants = tokens.filter((rec) => rec.type === "tenant");
    tenants.sort((a, b) => Number(b.created_at_ms || 0) - Number(a.created_at_ms || 0));
    state.adminTenantTokens = tenants;
    if (adminTenantList) {
      adminTenantList.hidden = false;
    }
    if (state.selectedAdminTokenID && !tenants.some((rec) => rec.token_id === state.selectedAdminTokenID)) {
      state.selectedAdminTokenID = "";
    }
    renderAdminTenantTokens();
    updateAdminCopyState();
    setAdminMessage(`Loaded ${tenants.length} tenant token(s).`);
  }

  async function revokeAdminToken(tokenID) {
    if (!tokenID) {
      return;
    }
    if (!window.confirm(`Revoke tenant token ${tokenID.slice(0, 8)}?`)) {
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
    for (const rec of state.adminTenantTokens) {
      if (rec.token_id === tokenID) {
        rec.revoked = true;
      }
    }
    renderAdminTenantTokens();
    setAdminMessage("Token revoked.");
  }

  async function createTenantTokens() {
    const token = tenantTokenInput.value.trim();
    if (!token) {
      setTenantMessage("Tenant token is required.", true);
      return;
    }
    if (!state.tenantVerified) {
      setTenantMessage("Verify tenant token first.", true);
      return;
    }
    const payload = {
      role: tenantRoleSelect.value,
    };
    const tenantID = tenantTenantInput.value.trim();
    if (!tenantID) {
      setTenantMessage("Tenant id is required.", true);
      return;
    }
    payload.tenant_id = tenantID;
    state.tenantToken = token;
    localStorage.setItem("tenant_token", state.tenantToken);
    setTenantMessage("Generating tokens...");
    let resp;
    try {
      resp = await tenantApi("/tenant/tokens", {
        method: "POST",
        body: JSON.stringify(payload),
      });
    } catch (err) {
      setTenantMessage(`Request failed: ${String(err)}`, true);
      return;
    }
    if (!resp.ok) {
      setTenantMessage(await resp.text(), true);
      return;
    }
    const result = await resp.json();
    state.lastTenantTokens = result;
    if (result.tenant_id) {
      tenantTenantInput.value = result.tenant_id;
    }
    tenantCopyUiTokenBtn.disabled = !(result.ui && result.ui.token);
    tenantCopyAgentTokenBtn.disabled = !(result.agent && result.agent.token);
    renderTenantResult(result);
    setTenantMessage("Tokens generated.");
  }

  async function copyTenantToken(kind) {
    const result = state.lastTenantTokens || {};
    const token = kind === "agent" ? result.agent && result.agent.token : result.ui && result.ui.token;
    if (!token) {
      return;
    }
    if (!navigator.clipboard || !navigator.clipboard.writeText) {
      setTenantMessage("Clipboard not available. Copy token from the result box.", true);
      return;
    }
    try {
      await navigator.clipboard.writeText(token);
      setTenantMessage(`${kind === "agent" ? "Agent" : "UI"} token copied to clipboard.`);
    } catch (_err) {
      setTenantMessage("Copy failed. Copy token from the result box.", true);
    }
  }

  function debounce(fn, ms) {
    let timer;
    return function (...args) {
      clearTimeout(timer);
      timer = setTimeout(() => fn.apply(this, args), ms);
    };
  }

  function switchAdminTab(tabName) {
    state.adminActiveTab = tabName;
    const tabs = document.querySelectorAll(".admin-tab");
    const panels = document.querySelectorAll(".admin-panel");
    for (const tab of tabs) {
      tab.classList.toggle("active", tab.dataset.tab === tabName);
    }
    for (const panel of panels) {
      panel.classList.toggle("active", panel.id === `adminPanel${tabName.charAt(0).toUpperCase() + tabName.slice(1)}`);
    }
    if (tabName === "overview") {
      refreshAdminOverview();
    } else if (tabName === "servers") {
      fetchAdminServers();
    } else if (tabName === "sessions") {
      fetchAdminSessions();
    } else if (tabName === "tokens") {
      if (!adminTenantList.hidden) {
        listAdminTenantTokens(false);
      }
    }
  }

  async function fetchAdminServers() {
    const resp = await adminApi("/admin/servers");
    if (!resp.ok) {
      return;
    }
    const body = await resp.json();
    state.adminServers = body.servers || [];
    renderAdminServers();
  }

  async function fetchAdminSessions() {
    const resp = await adminApi("/admin/sessions");
    if (!resp.ok) {
      return;
    }
    const body = await resp.json();
    state.adminSessions = body.sessions || [];
    renderAdminSessions();
  }

  async function refreshAdminOverview() {
    const [serversResp, sessionsResp, tokensResp] = await Promise.all([
      adminApi("/admin/servers"),
      adminApi("/admin/sessions"),
      adminApi("/admin/tokens"),
    ]);
    if (serversResp.ok) {
      const b = await serversResp.json();
      state.adminServers = b.servers || [];
    }
    if (sessionsResp.ok) {
      const b = await sessionsResp.json();
      state.adminSessions = b.sessions || [];
    }
    let allTokens = [];
    if (tokensResp.ok) {
      const b = await tokensResp.json();
      allTokens = b.tokens || [];
      const tenants = allTokens.filter((r) => r.type === "tenant");
      tenants.sort((a, b) => Number(b.created_at_ms || 0) - Number(a.created_at_ms || 0));
      state.adminTenantTokens = tenants;
    }
    renderAdminOverviewStats(allTokens);
  }

  function renderAdminOverviewStats(allTokens) {
    const servers = state.adminServers;
    const sessions = state.adminSessions;
    const tokens = allTokens || [];

    const online = servers.filter((s) => s.status === "online").length;
    const offline = servers.length - online;
    setText("statServersTotal", String(servers.length));
    setText("statServersOnline", `${online} online`);
    setText("statServersOffline", `${offline} offline`);

    const running = sessions.filter((s) => s.status === "running").length;
    const other = sessions.length - running;
    setText("statSessionsTotal", String(sessions.length));
    setText("statSessionsRunning", `${running} running`);
    setText("statSessionsOther", `${other} other`);

    const active = tokens.filter((t) => !t.revoked).length;
    const revoked = tokens.filter((t) => t.revoked).length;
    setText("statTokensTotal", String(tokens.length));
    setText("statTokensActive", `${active} active`);
    setText("statTokensRevoked", `${revoked} revoked`);

    const tenantIDs = new Set();
    for (const t of tokens) {
      if (t.tenant_id) tenantIDs.add(t.tenant_id);
    }
    for (const s of servers) {
      if (s.tenant_id) tenantIDs.add(s.tenant_id);
    }
    setText("statTenantsTotal", String(tenantIDs.size));
    setText("statTenantsDetail", "unique tenant IDs");
  }

  function setText(id, text) {
    const el = document.getElementById(id);
    if (el) el.textContent = text;
  }

  async function adminStopSession(sessionID) {
    if (!sessionID) return;
    if (!window.confirm(`Stop session ${sessionID.slice(0, 8)}?`)) return;
    const resp = await adminApi(`/admin/sessions/${encodeURIComponent(sessionID)}/stop`, {
      method: "POST",
      body: JSON.stringify({}),
    });
    if (!resp.ok) {
      alert(await resp.text());
      return;
    }
    await fetchAdminSessions();
  }

  function renderAdminServers() {
    const list = document.getElementById("adminServersList");
    if (!list) return;
    list.innerHTML = "";
    const query = (adminServersSearch && adminServersSearch.value || "").toLowerCase().trim();
    const filtered = state.adminServers.filter((s) => {
      if (!query) return true;
      return (s.server_id || "").toLowerCase().includes(query)
        || (s.hostname || "").toLowerCase().includes(query)
        || (s.tenant_id || "").toLowerCase().includes(query)
        || (s.os || "").toLowerCase().includes(query)
        || (s.arch || "").toLowerCase().includes(query);
    });
    if (!filtered.length) {
      renderEmptyItem(list, "No servers");
      return;
    }
    for (const s of filtered) {
      const li = document.createElement("li");
      li.className = "server-item";
      const statusClass = s.status === "online" ? "badge-online" : "badge-offline";
      const lastSeen = s.last_seen_ms ? formatTime(s.last_seen_ms) : "-";
      li.innerHTML = `
        <div class="server-main">
          <strong class="server-id">${escapeHtml(s.server_id)}</strong>
          <span class="badge ${statusClass}">${escapeHtml(s.status)}</span>
        </div>
        <div class="server-sub">
          <span class="server-host">${escapeHtml(s.hostname || "-")}</span>
          <span class="server-tags">${escapeHtml(s.os || "")}/${escapeHtml(s.arch || "")}</span>
          ${s.version ? `<span class="server-tags">v${escapeHtml(s.version)}</span>` : ""}
        </div>
        <div class="session-detail">
          <span>tenant: ${escapeHtml(s.tenant_id || "-")}</span>
          <span>seen: ${escapeHtml(lastSeen)}</span>
        </div>
      `;
      list.appendChild(li);
    }
  }

  function renderAdminSessions() {
    const list = document.getElementById("adminSessionsList");
    if (!list) return;
    list.innerHTML = "";
    const query = (adminSessionsSearch && adminSessionsSearch.value || "").toLowerCase().trim();
    const statusFilter = (adminSessionsStatusFilter && adminSessionsStatusFilter.value || "");
    const filtered = state.adminSessions.filter((s) => {
      if (statusFilter && s.status !== statusFilter) return false;
      if (!query) return true;
      return (s.session_id || "").toLowerCase().includes(query)
        || (s.server_id || "").toLowerCase().includes(query)
        || (s.tenant_id || "").toLowerCase().includes(query)
        || (s.cwd || "").toLowerCase().includes(query);
    });
    if (!filtered.length) {
      renderEmptyItem(list, "No sessions");
      return;
    }
    for (const s of filtered) {
      const li = document.createElement("li");
      li.className = "session-item";
      const statusBadge = s.status === "running" ? "badge badge-running" : "badge";
      li.innerHTML = `
        <div class="session-main">
          <strong class="session-id">${escapeHtml(s.session_id.slice(0, 8))}</strong>
          <div class="session-badges">
            <span class="${statusBadge}">${escapeHtml(s.status)}</span>
          </div>
        </div>
        <div class="session-sub">${escapeHtml(s.cwd || "-")}</div>
        <div class="session-detail">
          <span>server: ${escapeHtml(s.server_id || "-")}</span>
          <span>tenant: ${escapeHtml(s.tenant_id || "-")}</span>
        </div>
        ${s.status === "running" ? `<div class="session-actions"><button type="button" data-session-id="${escapeHtml(s.session_id)}" class="btn-danger admin-stop-btn">Stop</button></div>` : ""}
      `;
      const stopBtn = li.querySelector(".admin-stop-btn");
      if (stopBtn) {
        stopBtn.addEventListener("click", (e) => {
          e.stopPropagation();
          adminStopSession(s.session_id);
        });
      }
      list.appendChild(li);
    }
  }

  function api(path, init = {}) {
    const headers = new Headers(init.headers || {});
    headers.set("Authorization", `Bearer ${state.token}`);
    if (init.body && !headers.has("Content-Type")) {
      headers.set("Content-Type", "application/json");
    }
    return fetch(path, { ...init, headers });
  }

  function tenantApi(path, init = {}) {
    const headers = new Headers(init.headers || {});
    headers.set("Authorization", `Bearer ${state.tenantToken}`);
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

  if (isControllerPage) {
    connectWS();
    refreshAll();
  }
})();
