import { createUIApi } from "../shared/http.js";
import { createSidebarController } from "../shared/sidebar.js";
import { createWSClient, setWSBadge } from "../shared/ws.js";
import { createTerminalController } from "../controller/terminal.js";
import { createWorkspaceShell } from "../shared/workspace-shell.js";
import {
  WINDOWS_PTY_UNSUPPORTED_ERROR,
  bytesToB64,
  escapeHtml,
  parseEnv,
} from "../shared/utils.js";
import { renderServerList } from "../shared/server-list.js";
import { renderSessionList } from "../shared/session-list.js";
import { renderChatMarkdown } from "../chat/markdown.js";

export function initWorkspacePage() {
  const MAX_SCREENSHOTS_PER_MESSAGE = 3;
  const MAX_SCREENSHOT_BYTES = 900 * 1024;
  const MAX_SCREENSHOT_EDGE = 1800;
  const SLOW_RESPONSE_MS = 12000;
  const query = new URLSearchParams(window.location.search);

  const state = {
    token: localStorage.getItem("ui_token") || "admin-dev-token",
    ws: null,
    servers: [],
    sessions: [],
    approvals: new Map(),
    selectedServerID: query.get("server_id") || "",
    requestedSessionID: query.get("session_id") || "",
    selectedSessionID: query.get("session_id") || "",
    currentView: query.get("view") === "chat" ? "chat" : "pty",
    switchingSessionID: "",
    pendingFirstOutputSessionID: "",
    instanceHistory: [],
    chatMessages: [],
    pendingScreenshots: [],
    pendingTurns: 0,
    pendingSlowTimer: null,
  };
  const e2eState = {
    lastTermInText: "",
    lastTermInAtMs: 0,
    lastTermInSessionID: "",
    termInCount: 0,
    termInRecentText: "",
    lastTermOutAtMs: 0,
    lastTermOutSessionID: "",
    termOutCount: 0,
    termOutRecentText: "",
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
  const newSessionBtn = document.getElementById("newSessionBtn");
  const chatContainer = document.getElementById("chatContainer");
  const terminalSection = document.getElementById("terminalSection");
  const chatMessagesEl = document.getElementById("chatMessages");
  const chatInput = document.getElementById("chatInput");
  const chatSendBtn = document.getElementById("chatSendBtn");
  const chatAttachBtn = document.getElementById("chatAttachBtn");
  const chatImageInput = document.getElementById("chatImageInput");
  const chatAttachmentBar = document.getElementById("chatAttachmentBar");
  const chatAttachmentList = document.getElementById("chatAttachmentList");
  const chatRunState = document.getElementById("chatRunState");

  function getSelectedSession() {
    return state.sessions.find((s) => s.session_id === state.selectedSessionID) || null;
  }

  function getServerByID(serverID) {
    if (!serverID) return null;
    return state.servers.find((s) => s.server_id === serverID) || null;
  }

  function getSelectedServer() {
    return getServerByID(state.selectedServerID);
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

  function sendWS(msg) {
    if (msg?.type === "term_in") {
      const chunk = decodeB64Text(msg.data_b64);
      e2eState.lastTermInText = chunk;
      e2eState.lastTermInAtMs = Date.now();
      e2eState.lastTermInSessionID = String(msg.session_id || "");
      e2eState.termInCount += 1;
      e2eState.termInRecentText = `${e2eState.termInRecentText}${chunk}`.slice(-512);
    }
    return wsClient.send(msg);
  }

  function sendTerminalInput(text) {
    if (!state.selectedSessionID) return false;
    return sendWS({
      type: "term_in",
      session_id: state.selectedSessionID,
      data_b64: bytesToB64(text),
    });
  }

  const terminal = createTerminalController({
    getSelectedSessionID: () => state.selectedSessionID,
    sendWS,
    onTermData: () => {},
  });

  const sidebar = createSidebarController({
    isControllerPage: true,
    isChatPage: true,
    onLayoutChange: () => terminal.syncLayout(),
    approvalDetails,
  });

  const workspace = createWorkspaceShell({
    viewMode: state.currentView,
    getSelectedSession,
    onOpenTerminalView: (session) => openView("pty", session.session_id),
    onOpenChatView: (session) => openView("chat", session.session_id),
    onSwitchMode: (session) => switchSessionTo(state.currentView, session),
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
      attachSelectedView(send);
    },
    onMessage: (msg) => handleWS(msg),
    onError: (e) => console.error("[ws] error", e),
  });

  function syncQuery() {
    const nextQuery = new URLSearchParams();
    if (state.selectedServerID) nextQuery.set("server_id", state.selectedServerID);
    if (state.selectedSessionID) nextQuery.set("session_id", state.selectedSessionID);
    if (state.currentView === "chat") nextQuery.set("view", "chat");
    const next = nextQuery.toString();
    const target = next ? `/?${next}` : "/";
    window.history.replaceState({}, "", target);
  }

  function updateViewVisibility() {
    const showingChat = state.currentView === "chat";
    if (chatContainer) chatContainer.hidden = !showingChat;
    if (terminalSection) terminalSection.hidden = showingChat;
    workspace.setViewMode(state.currentView);
  }

  function isWindowsServer(server) {
    return String((server && server.os) || "").toLowerCase() === "windows";
  }

  function ensureSupportedViewForServer(server) {
    if (!isWindowsServer(server) || state.currentView !== "pty") return false;
    state.currentView = "chat";
    updateViewVisibility();
    syncQuery();
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

  function clearPendingSlowTimer() {
    if (!state.pendingSlowTimer) return;
    clearTimeout(state.pendingSlowTimer);
    state.pendingSlowTimer = null;
  }

  function setRunState(kind, text) {
    if (!chatRunState) return;
    const content = String(text || "").trim();
    if (!content) {
      chatRunState.hidden = true;
      chatRunState.className = "chat-run-state";
      chatRunState.textContent = "";
      return;
    }
    chatRunState.hidden = false;
    chatRunState.className = `chat-run-state ${kind || ""}`.trim();
    chatRunState.textContent = content;
  }

  function beginPendingTurn() {
    state.pendingTurns += 1;
    if (state.pendingTurns > 1) {
      setRunState("running", `Running... (${state.pendingTurns} requests pending)`);
      return;
    }
    setRunState("running", "Running... waiting for assistant response");
    clearPendingSlowTimer();
    state.pendingSlowTimer = setTimeout(() => {
      if (state.pendingTurns > 0) {
        setRunState("slow", "Still running... this is taking longer than usual");
      }
    }, SLOW_RESPONSE_MS);
  }

  function completePendingTurn() {
    if (state.pendingTurns <= 0) return;
    state.pendingTurns -= 1;
    if (state.pendingTurns > 0) {
      setRunState("running", `Running... (${state.pendingTurns} requests pending)`);
      return;
    }
    clearPendingSlowTimer();
    setRunState("", "");
  }

  function failPendingTurns(reason) {
    if (state.pendingTurns <= 0) return;
    state.pendingTurns = 0;
    clearPendingSlowTimer();
    setRunState("error", `Execution failed: ${reason || "unknown error"}`);
  }

  function estimateDataURLBytes(dataURL) {
    const idx = dataURL.indexOf(",");
    if (idx <= 0) return 0;
    const b64 = dataURL.slice(idx + 1);
    const padding = b64.endsWith("==") ? 2 : b64.endsWith("=") ? 1 : 0;
    return Math.floor((b64.length * 3) / 4) - padding;
  }

  function readFileAsDataURL(file) {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result || ""));
      reader.onerror = () => reject(new Error("read_failed"));
      reader.readAsDataURL(file);
    });
  }

  function loadImage(dataURL) {
    return new Promise((resolve, reject) => {
      const img = new Image();
      img.onload = () => resolve(img);
      img.onerror = () => reject(new Error("image_decode_failed"));
      img.src = dataURL;
    });
  }

  async function optimizeScreenshot(file) {
    const sourceDataURL = await readFileAsDataURL(file);
    const img = await loadImage(sourceDataURL);
    const scale = Math.min(1, MAX_SCREENSHOT_EDGE / Math.max(img.naturalWidth, img.naturalHeight));
    const width = Math.max(1, Math.round(img.naturalWidth * scale));
    const height = Math.max(1, Math.round(img.naturalHeight * scale));

    const canvas = document.createElement("canvas");
    canvas.width = width;
    canvas.height = height;
    const ctx = canvas.getContext("2d");
    if (!ctx) throw new Error("canvas_not_supported");
    ctx.drawImage(img, 0, 0, width, height);

    let dataURL = canvas.toDataURL(file.type || "image/png");
    if (estimateDataURLBytes(dataURL) > MAX_SCREENSHOT_BYTES) {
      dataURL = canvas.toDataURL("image/jpeg", 0.85);
    }
    if (estimateDataURLBytes(dataURL) > MAX_SCREENSHOT_BYTES) {
      dataURL = canvas.toDataURL("image/jpeg", 0.72);
    }
    if (estimateDataURLBytes(dataURL) > MAX_SCREENSHOT_BYTES) {
      throw new Error("image_too_large");
    }
    return dataURL;
  }

  function renderPendingScreenshots() {
    if (!chatAttachmentBar || !chatAttachmentList) return;
    chatAttachmentList.innerHTML = "";
    if (!state.pendingScreenshots.length) {
      chatAttachmentBar.hidden = true;
      return;
    }
    chatAttachmentBar.hidden = false;
    for (let i = 0; i < state.pendingScreenshots.length; i += 1) {
      const entry = state.pendingScreenshots[i];
      const item = document.createElement("div");
      item.className = "chat-attachment-item";
      item.title = entry.name;

      const img = document.createElement("img");
      img.src = entry.dataURL;
      img.alt = entry.name || `screenshot-${i + 1}`;
      item.appendChild(img);

      const removeBtn = document.createElement("button");
      removeBtn.type = "button";
      removeBtn.className = "chat-attachment-remove";
      removeBtn.textContent = "×";
      removeBtn.title = "Remove";
      removeBtn.addEventListener("click", () => {
        state.pendingScreenshots.splice(i, 1);
        renderPendingScreenshots();
      });
      item.appendChild(removeBtn);
      chatAttachmentList.appendChild(item);
    }
  }

  async function addScreenshotFiles(files) {
    const fileList = Array.isArray(files) ? files : Array.from(files || []);
    if (!fileList.length) return;

    const slots = MAX_SCREENSHOTS_PER_MESSAGE - state.pendingScreenshots.length;
    if (slots <= 0) {
      alert(`Only ${MAX_SCREENSHOTS_PER_MESSAGE} screenshots per message`);
      return;
    }

    const accepted = fileList.filter((f) => f && /^image\//i.test(f.type)).slice(0, slots);
    for (const file of accepted) {
      try {
        const dataURL = await optimizeScreenshot(file);
        state.pendingScreenshots.push({
          name: file.name || "screenshot",
          dataURL,
        });
      } catch (err) {
        console.error("[workspace] screenshot failed", err);
        alert(`Failed to attach ${file.name || "image"} (too large or invalid image)`);
      }
    }
    renderPendingScreenshots();
  }

  function getChatOperations(meta) {
    if (!meta || typeof meta !== "object" || !Array.isArray(meta.operations)) return [];
    const operations = [];
    for (const op of meta.operations) {
      if (typeof op !== "string") continue;
      const text = op.trim();
      if (!text) continue;
      operations.push(text);
    }
    return operations;
  }

  function formatInstanceShort(instanceID) {
    const value = String(instanceID || "").trim();
    return value ? value.slice(0, 8) : "-";
  }

  function getChatContentParts(meta) {
    if (!meta || typeof meta !== "object" || !Array.isArray(meta.content_parts)) return [];
    const parts = [];
    for (const part of meta.content_parts) {
      if (!part || typeof part !== "object") continue;
      if (part.type === "text" && typeof part.text === "string" && part.text.trim()) {
        parts.push({ type: "text", text: part.text });
        continue;
      }
      if (part.type !== "image") continue;
      const source = part.source;
      if (!source || typeof source !== "object") continue;
      if (source.type !== "base64") continue;
      if (typeof source.media_type !== "string" || !/^image\//i.test(source.media_type)) continue;
      if (typeof source.data !== "string" || !source.data.trim()) continue;
      parts.push({
        type: "image",
        source: {
          type: "base64",
          media_type: source.media_type,
          data: source.data,
        },
      });
    }
    return parts;
  }

  function buildMarkdownFromParts(parts, fallbackText) {
    if (!Array.isArray(parts) || !parts.length) return fallbackText || "";
    const lines = [];
    for (const part of parts) {
      if (part.type === "text") {
        lines.push(part.text);
        continue;
      }
      if (part.type === "image" && part.source) {
        const mediaType = String(part.source.media_type || "").trim();
        const data = String(part.source.data || "").trim();
        if (!mediaType || !data) continue;
        lines.push(`![screenshot](data:${mediaType};base64,${data})`);
      }
    }
    return lines.join("\n\n") || fallbackText || "";
  }

  function isProgressMessage(chatMsg) {
    const meta = chatMsg && typeof chatMsg === "object" ? chatMsg.meta : null;
    if (!meta || typeof meta !== "object") return false;
    return meta.source === "claude-stream-json" && meta.progress === true;
  }

  function renderChatMessages() {
    if (!chatMessagesEl) return;
    chatMessagesEl.innerHTML = "";
    if (!state.chatMessages.length) {
      const empty = document.createElement("div");
      empty.className = "chat-empty";
      if (!state.selectedSessionID) {
        empty.textContent = "Select a session";
      } else if ((getSelectedSession()?.session_type || "") !== "chat") {
        empty.textContent = 'This session is currently in Terminal mode. Use "Switch to Chat" to continue the shared conversation here.';
      } else {
        empty.textContent = "No messages yet";
      }
      chatMessagesEl.appendChild(empty);
      return;
    }

    for (const m of state.chatMessages) {
      const bubble = document.createElement("div");
      bubble.className = `chat-bubble ${m.role === "user" ? "user" : "assistant"}`;
      if (m.role === "assistant" && isProgressMessage(m)) {
        bubble.classList.add("progress");
      }
      const body = document.createElement("div");
      body.className = "chat-markdown";
      const markdown = buildMarkdownFromParts(getChatContentParts(m.meta), m.content);
      body.innerHTML = renderChatMarkdown(markdown);
      bubble.appendChild(body);

      if (m.role === "assistant") {
        const operations = getChatOperations(m.meta);
        if (operations.length) {
          const opsBox = document.createElement("div");
          opsBox.className = "chat-bubble-ops";
          const opsTitle = document.createElement("div");
          opsTitle.className = "chat-bubble-ops-title";
          opsTitle.textContent = "Intermediate steps";
          opsBox.appendChild(opsTitle);
          for (let i = 0; i < operations.length; i += 1) {
            const item = document.createElement("div");
            item.className = "chat-bubble-op-item";
            item.textContent = `${i + 1}. ${operations[i]}`;
            opsBox.appendChild(item);
          }
          bubble.appendChild(opsBox);
        }
      }

      const meta = document.createElement("div");
      meta.className = "chat-bubble-meta";
      meta.textContent = `${new Date(m.ts_ms).toLocaleTimeString()} • instance ${formatInstanceShort(m.instance_id)}`;
      bubble.appendChild(meta);
      chatMessagesEl.appendChild(bubble);
    }

    chatMessagesEl.scrollTop = chatMessagesEl.scrollHeight;
  }

  function renderServers() {
    renderServerList(serversList, state.servers, state.selectedServerID, async (server) => {
      state.selectedServerID = server.server_id;
      renderServers();
      ensureSupportedViewForServer(server);
      await fetchSessions();
      syncQuery();
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
    workspace.render(getSelectedSession());
  }

  function renderInstanceHistory() {
    if (!instanceHistoryList || !instanceHistoryCount) return;
    instanceHistoryList.innerHTML = "";
    const session = getSelectedSession();
    const items = state.instanceHistory || [];
    instanceHistoryCount.textContent = String(items.length);
    if (!state.selectedSessionID) {
      const li = document.createElement("li");
      li.className = "session-item instance-item";
      li.textContent = "Select a session";
      instanceHistoryList.appendChild(li);
      return;
    }
    if (!items.length) {
      const li = document.createElement("li");
      li.className = "session-item instance-item";
      li.textContent = "No instances";
      instanceHistoryList.appendChild(li);
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
      instanceHistoryList.appendChild(li);
    }
  }

  function renderApprovals() {
    if (!approvalList || !approvalCount) return;
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
    ensureSupportedViewForServer(getSelectedServer());
    renderServers();
  }

  async function fetchSessions() {
    const q = state.selectedServerID ? `?server_id=${encodeURIComponent(state.selectedServerID)}` : "";
    const resp = await api(`/api/sessions${q}`);
    if (!resp.ok) return;
    state.sessions = (await resp.json()).sessions || [];
    renderSessions();
    renderInstanceHistory();

    if (state.requestedSessionID && !state.selectedSessionID) {
      const target = state.sessions.find((s) => s.session_id === state.requestedSessionID);
      if (target) {
        state.requestedSessionID = "";
        await attachSession(target.session_id);
        return;
      }
    }

    if (state.selectedSessionID && !state.sessions.some((s) => s.session_id === state.selectedSessionID)) {
      state.selectedSessionID = "";
      state.pendingFirstOutputSessionID = "";
      state.instanceHistory = [];
      state.chatMessages = [];
      state.pendingScreenshots = [];
      terminal.clearSession();
      renderInstanceHistory();
      renderChatMessages();
      renderPendingScreenshots();
      workspace.render(null);
      syncQuery();
      return;
    }

    const active = getSelectedSession();
    if (active && (active.session_type || "pty") === "pty") {
      terminal.setSessionInfo(active.session_id, active.active_instance_id || "", state.pendingFirstOutputSessionID === active.session_id);
    } else if (!active || state.currentView === "pty") {
      terminal.clearSession();
    }
    workspace.render(active);
  }

  async function refreshAll() {
    await fetchServers();
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

  async function loadChatHistory(sessionID) {
    try {
      const resp = await api(`/api/sessions/${encodeURIComponent(sessionID)}/chat`);
      if (!resp.ok) return;
      if (state.selectedSessionID !== sessionID) return;
      state.chatMessages = (await resp.json()).messages || [];
      renderChatMessages();
    } catch (e) {
      console.error("[workspace] failed to load chat history", e);
    }
  }

  function attachSelectedView(send = sendWS) {
    const session = getSelectedSession();
    if (!session) return;
    if (state.currentView === "pty") {
      if ((session.session_type || "pty") !== "pty") {
        state.pendingFirstOutputSessionID = "";
        terminal.clearSession();
        return;
      }
      state.pendingFirstOutputSessionID = session.session_id;
      terminal.resetForSession(session.session_id);
      send({ type: "attach", data: { session_id: session.session_id, since_seq: 0 } });
      terminal.sendResize();
      return;
    }
    state.pendingFirstOutputSessionID = "";
    terminal.setSessionInfo(session.session_id, session.active_instance_id || "", false);
    if ((session.session_type || "pty") !== "chat") return;
    send({ type: "attach", data: { session_id: session.session_id, since_seq: 0 } });
  }

  async function syncSelectedSession() {
    const session = getSelectedSession();
    workspace.render(session);
    if (!session) {
      renderChatMessages();
      renderPendingScreenshots();
      renderInstanceHistory();
      terminal.clearSession();
      return;
    }
    fetchSessionInstances(session.session_id);
    if (state.currentView === "pty") {
      state.chatMessages = [];
      renderChatMessages();
      attachSelectedView();
    } else {
      terminal.setSessionInfo(session.session_id, session.active_instance_id || "", false);
      attachSelectedView();
      if ((session.session_type || "pty") === "chat") {
        await loadChatHistory(session.session_id);
      } else {
        state.chatMessages = [];
        renderChatMessages();
      }
    }
  }

  async function openView(view, sessionID = state.selectedSessionID) {
    state.currentView = view === "chat" ? "chat" : "pty";
    if (sessionID) state.selectedSessionID = sessionID;
    updateViewVisibility();
    syncQuery();
    renderSessions();
    await syncSelectedSession();
  }

  async function attachSession(sessionID) {
    if (!sessionID) return;
    state.selectedSessionID = sessionID;
    state.pendingScreenshots = [];
    state.pendingTurns = 0;
    clearPendingSlowTimer();
    setRunState("", "");
    renderPendingScreenshots();
    renderSessions();
    syncQuery();
    await syncSelectedSession();
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
      state.chatMessages = [];
      state.pendingScreenshots = [];
      renderInstanceHistory();
      renderChatMessages();
      renderPendingScreenshots();
      terminal.clearSession();
      workspace.render(null);
      syncQuery();
    }
    for (const [eventID, approval] of state.approvals.entries()) {
      if (approval.session_id === session.session_id) state.approvals.delete(eventID);
    }
    renderApprovals();
    await fetchSessions();
  }

  async function switchSessionTo(targetView, source) {
    if (!source?.session_id) return;
    if (state.switchingSessionID) return;
    const serverID = source.server_id || state.selectedServerID;
    if (!serverID) return alert("select a server first");

    if (targetView === "pty" && isWindowsServer(getServerByID(serverID))) {
      alert("PTY is not supported on Windows yet.");
      return;
    }

    if ((source.session_type || "pty") === targetView) {
      await openView(targetView, source.session_id);
      return;
    }

    state.switchingSessionID = source.session_id;
    renderSessions();
    try {
      const body = {
        session_type: targetView,
        env: parseEnv(envInput.value),
      };
      if (targetView === "pty") {
        const term = terminal.getTerm();
        body.cols = term?.cols || 120;
        body.rows = term?.rows || 30;
      }
      const resp = await api(`/api/sessions/${encodeURIComponent(source.session_id)}/switch`, {
        method: "POST",
        body: JSON.stringify(body),
      });
      if (!resp.ok) return alert(formatSessionCreateError(await resp.text()));
      await fetchSessions();
      await openView(targetView, source.session_id);
    } finally {
      state.switchingSessionID = "";
      renderSessions();
    }
  }

  function handleWS(msg) {
    if (msg.type === "term_out" && msg.session_id === state.selectedSessionID && msg.data_b64) {
      const chunk = decodeB64Text(msg.data_b64);
      e2eState.lastTermOutAtMs = Date.now();
      e2eState.lastTermOutSessionID = String(msg.session_id || "");
      e2eState.termOutCount += 1;
      e2eState.termOutRecentText = `${e2eState.termOutRecentText}${chunk}`.slice(-2048);
      terminal.writeOutput(msg, state.pendingFirstOutputSessionID, () => {
        state.pendingFirstOutputSessionID = "";
        terminal.markLoaded(msg.session_id, msg.instance_id || "");
      });
      return;
    }

    if (msg.type === "chat_msg" && msg.data) {
      const cm = msg.data;
      if (cm.session_id === state.selectedSessionID) {
        state.chatMessages.push(cm);
        if (cm.role === "assistant" && !isProgressMessage(cm)) {
          completePendingTurn();
        }
        renderChatMessages();
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

    if (msg.type === "session_update" && msg.data) {
      const data = msg.data;
      if (data.session_id === state.selectedSessionID && (data.status === "error" || data.status === "exited")) {
        failPendingTurns(data.exit_reason || `session ${data.status}`);
      }
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

  function sendChatMessage() {
    if (!chatInput) return;
    const text = chatInput.value.trim();
    if (!text && !state.pendingScreenshots.length) return;
    if (!state.selectedSessionID) {
      alert("Select or create a chat session first");
      return;
    }
    if ((getSelectedSession()?.session_type || "") !== "chat") {
      alert("This session is currently in Terminal mode. Switch to Chat first.");
      return;
    }
    const contentParts = [];
    if (text) {
      contentParts.push({ type: "text", text });
    }
    for (const entry of state.pendingScreenshots) {
      const m = String(entry.dataURL || "").match(/^data:(image\/[a-z0-9.+-]+);base64,(.+)$/i);
      if (!m) continue;
      contentParts.push({
        type: "image",
        source: {
          type: "base64",
          media_type: m[1].toLowerCase(),
          data: m[2],
        },
      });
    }
    const sent = sendWS({
      type: "chat_in",
      session_id: state.selectedSessionID,
      data: {
        content: text,
        content_parts: contentParts,
      },
    });
    if (!sent) {
      setRunState("error", "Failed to send: WebSocket disconnected");
      return;
    }
    beginPendingTurn();
    chatInput.value = "";
    chatInput.style.height = "auto";
    state.pendingScreenshots = [];
    renderPendingScreenshots();
  }

  async function createNewSession() {
    if (!state.selectedServerID) return alert("select a server first");
    const server = getSelectedServer();
    ensureSupportedViewForServer(server);

    const cwd = cwdInput.value.trim();
    if (!cwd) return alert("cwd is required");

    const customSessionID = sessionIDInput.value.trim().toLowerCase();
    const body = {
      server_id: state.selectedServerID,
      cwd,
      env: parseEnv(envInput.value),
    };
    if (customSessionID) body.session_id = customSessionID;

    if (state.currentView === "chat") {
      body.session_type = "chat";
    } else {
      const term = terminal.getTerm();
      body.cols = term?.cols || 120;
      body.rows = term?.rows || 30;
    }

    const resp = await api("/api/sessions", { method: "POST", body: JSON.stringify(body) });
    if (!resp.ok) return alert(formatSessionCreateError(await resp.text()));

    const session = await resp.json();
    await fetchSessions();
    await attachSession(session.session_id);
  }

  terminal.init();
  terminal.bindToolbarKeys();
  sidebar.mount();
  window.__CC_E2E__ = {
    sendTerminalInput,
    getTerminalIOState: () => ({ ...e2eState }),
  };
  updateViewVisibility();
  renderInstanceHistory();
  renderApprovals();
  renderChatMessages();
  renderPendingScreenshots();
  workspace.render(null);

  tokenInput.value = state.token;
  saveTokenBtn?.addEventListener("click", () => applyUIToken(tokenInput.value.trim()));
  document.getElementById("refreshServersBtn")?.addEventListener("click", fetchServers);
  document.getElementById("refreshSessionsBtn")?.addEventListener("click", fetchSessions);
  newSessionBtn?.addEventListener("click", createNewSession);
  chatSendBtn?.addEventListener("click", sendChatMessage);
  chatAttachBtn?.addEventListener("click", () => chatImageInput?.click());
  chatImageInput?.addEventListener("change", async () => {
    await addScreenshotFiles(chatImageInput.files);
    chatImageInput.value = "";
  });

  if (chatInput) {
    chatInput.addEventListener("keydown", (e) => {
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        sendChatMessage();
      }
    });
    chatInput.addEventListener("input", () => {
      chatInput.style.height = "auto";
      chatInput.style.height = `${Math.min(chatInput.scrollHeight, 120)}px`;
    });
    chatInput.addEventListener("paste", async (e) => {
      const items = Array.from(e.clipboardData?.items || []);
      const imageFiles = [];
      for (const item of items) {
        if (item.kind === "file" && /^image\//i.test(item.type)) {
          const file = item.getAsFile();
          if (file) imageFiles.push(file);
        }
      }
      if (!imageFiles.length) return;
      e.preventDefault();
      await addScreenshotFiles(imageFiles);
    });
  }

  if (localStorage.getItem("ui_token")) {
    wsClient.connect();
    refreshAll();
  }
}
