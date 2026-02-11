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
    selectedServerID: "",
    selectedSessionID: "",
    ws: null,
    approvals: new Map(),
    sessions: [],
    servers: [],
  };

  const tokenInput = document.getElementById("tokenInput");
  const saveTokenBtn = document.getElementById("saveTokenBtn");
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

  tokenInput.value = state.token;

  const term = new Terminal({
    cursorBlink: true,
    convertEol: true,
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
    state.token = tokenInput.value.trim();
    localStorage.setItem("ui_token", state.token);
    reconnectWS();
    refreshAll();
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

  document.getElementById("scrollUp").addEventListener("click", () => term.scrollPages(-1));
  document.getElementById("scrollDown").addEventListener("click", () => term.scrollPages(1));
  document.getElementById("keyUp").addEventListener("click", () => sendQuickKey("\x1b[A"));
  document.getElementById("keyDown").addEventListener("click", () => sendQuickKey("\x1b[B"));
  document.getElementById("keyRight").addEventListener("click", () => sendQuickKey("\x1b[C"));
  document.getElementById("keyLeft").addEventListener("click", () => sendQuickKey("\x1b[D"));
  document.getElementById("keyEnter").addEventListener("click", () => sendQuickKey("\r"));

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

  function renderServers() {
    serversList.innerHTML = "";
    for (const s of state.servers) {
      const li = document.createElement("li");
      if (s.server_id === state.selectedServerID) li.classList.add("selected");
      const statusClass = s.status === "online" ? "badge-online" : "badge-offline";
      li.innerHTML = `
        <div class="row">
          <strong>${escapeHtml(s.server_id)}</strong>
          <span class="badge ${statusClass}">${escapeHtml(s.status)}</span>
        </div>
        <div class="item-meta">${escapeHtml(s.hostname || "")}</div>
        <div class="item-meta">${(s.tags || []).map(escapeHtml).join(", ")}</div>
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
    for (const s of state.sessions) {
      const li = document.createElement("li");
      if (s.session_id === state.selectedSessionID) li.classList.add("selected");
      const statusBadge = s.status === "running" ? "badge badge-running" : "badge";
      li.innerHTML = `
        <div class="row">
          <strong>${escapeHtml(s.session_id.slice(0, 8))}</strong>
          <span class="${statusBadge}">${escapeHtml(s.status)}</span>
        </div>
        <div class="item-meta">${escapeHtml(s.cwd || "")}</div>
        ${s.resume_id ? `<div class="item-meta">resume: ${escapeHtml(s.resume_id)}</div>` : ""}
        <div class="item-meta">approval: ${s.awaiting_approval ? "yes" : "no"}</div>
        ${s.resume_id ? `<div class="row" style="margin-top:6px;"><button type="button" data-action="resume" class="btn-secondary">Resume</button></div>` : ""}
        ${s.exit_reason ? `<div class="item-meta">reason: ${escapeHtml(s.exit_reason)}</div>` : ""}
      `;
      if (s.resume_id) {
        const resumeBtn = li.querySelector('[data-action="resume"]');
        resumeBtn.addEventListener("click", async (e) => {
          e.stopPropagation();
          await resumeSession(s);
        });
      }
      li.addEventListener("click", () => attachSession(s.session_id));
      sessionsList.appendChild(li);
    }
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
        term.write(b64ToBytes(msg.data_b64));
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
    state.selectedSessionID = sessionID;
    currentSessionLabel.textContent = `Session: ${sessionID}`;
    renderSessions();
    term.clear();
    sendWS({
      type: "attach",
      data: { session_id: sessionID, since_seq: 0 },
    });
    sendResize();
    closeSidebarOnMobile();
  }

  function action(kind, eventID = "", sessionID = "") {
    const target = sessionID || state.selectedSessionID;
    if (!target) {
      return;
    }
    sendWS({
      type: "action",
      session_id: target,
      // Rely on server-side current pending approval for this session.
      // This avoids stale event_id mismatches after reconnect/replay.
      data: { kind },
    });
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

  function api(path, init = {}) {
    const headers = new Headers(init.headers || {});
    headers.set("Authorization", `Bearer ${state.token}`);
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

