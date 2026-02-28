import { createUIApi } from "../shared/http.js";
import { createSidebarController } from "../shared/sidebar.js";
import { createWSClient, setWSBadge } from "../shared/ws.js";
import { escapeHtml, parseEnv } from "../shared/utils.js";
import { renderServerList } from "../shared/server-list.js";
import { renderSessionList } from "../shared/session-list.js";
import { createWorkspaceShell } from "../shared/workspace-shell.js";
import { renderChatMarkdown } from "./markdown.js";

export function initChatPage() {
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
    selectedServerID: query.get("server_id") || "",
    requestedSessionID: query.get("session_id") || "",
    selectedChatSessionID: query.get("session_id") || "",
    switchingSessionID: "",
    chatMessages: [],
    pendingScreenshots: [],
    pendingTurns: 0,
    pendingSlowTimer: null,
    instanceHistory: [],
  };

  const api = createUIApi(() => state.token);

  const tokenInput = document.getElementById("tokenInput");
  const saveTokenBtn = document.getElementById("saveTokenBtn");
  const serversList = document.getElementById("serversList");
  const sessionsList = document.getElementById("sessionsList");
  const cwdInput = document.getElementById("cwdInput");
  const envInput = document.getElementById("envInput");
  const refreshServersBtn = document.getElementById("refreshServersBtn");
  const refreshSessionsBtn = document.getElementById("refreshSessionsBtn");
  const newChatBtn = document.getElementById("newChatBtn");
  const chatMessagesEl = document.getElementById("chatMessages");
  const chatInput = document.getElementById("chatInput");
  const chatSendBtn = document.getElementById("chatSendBtn");
  const chatAttachBtn = document.getElementById("chatAttachBtn");
  const chatImageInput = document.getElementById("chatImageInput");
  const chatAttachmentBar = document.getElementById("chatAttachmentBar");
  const chatAttachmentList = document.getElementById("chatAttachmentList");
  const chatInstanceHistoryList = document.getElementById("chatInstanceHistoryList");
  const chatInstanceHistoryCount = document.getElementById("chatInstanceHistoryCount");
  const chatRunState = document.getElementById("chatRunState");

  function getSelectedSession() {
    return state.sessions.find((s) => s.session_id === state.selectedChatSessionID) || null;
  }

  const sidebar = createSidebarController({
    isControllerPage: false,
    isChatPage: true,
    onLayoutChange: () => {},
    approvalDetails: document.getElementById("approvalDetails"),
  });

  const workspace = createWorkspaceShell({
    viewMode: "chat",
    getSelectedSession,
    onOpenTerminalView: (session) => openTerminalView(session),
    onOpenChatView: (session) => attachChatSession(session.session_id),
    onSwitchMode: (session) => switchSessionToChat(session),
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
      if (state.selectedChatSessionID && (getSelectedSession()?.session_type || "pty") === "chat") {
        send({ type: "attach", data: { session_id: state.selectedChatSessionID, since_seq: 0 } });
      }
    },
    onMessage: (msg, { send: _send }) => {
      if (msg.type === "chat_msg" && msg.data) {
        const cm = msg.data;
        if (cm.session_id === state.selectedChatSessionID) {
          state.chatMessages.push(cm);
          if (cm.role === "assistant" && !isProgressMessage(cm)) {
            completePendingTurn();
          }
          renderChatMessages();
        }
        return;
      }
      if (msg.type === "session_update" && msg.data) {
        const data = msg.data;
        if (data.session_id === state.selectedChatSessionID && (data.status === "error" || data.status === "exited")) {
          failPendingTurns(data.exit_reason || `session ${data.status}`);
        }
        fetchSessions();
        if (data.session_id === state.selectedChatSessionID) {
          fetchSessionInstances(state.selectedChatSessionID);
        }
      }
    },
    onError: (e) => console.error("[ws] error", e),
  });

  function sendWS(msg) {
    return wsClient.send(msg);
  }

  function getServerByID(serverID) {
    if (!serverID) return null;
    return state.servers.find((s) => s.server_id === serverID) || null;
  }

  function isWindowsServer(server) {
    return String((server && server.os) || "").toLowerCase() === "windows";
  }

  function formatSessionCreateError(rawText) {
    const msg = String(rawText || "").trim();
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

  function isProgressMessage(chatMsg) {
    const meta = chatMsg && typeof chatMsg === "object" ? chatMsg.meta : null;
    if (!meta || typeof meta !== "object") return false;
    return meta.source === "claude-stream-json" && meta.progress === true;
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
    if (state.pendingTurns <= 0) {
      return;
    }
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
        console.error("[chat] screenshot failed", err);
        alert(`Failed to attach ${file.name || "image"} (too large or invalid image)`);
      }
    }
    renderPendingScreenshots();
  }

  function applyUIToken(token) {
    state.token = token || "";
    if (tokenInput) tokenInput.value = state.token;
    localStorage.setItem("ui_token", state.token);
    wsClient.reconnect();
    refreshAll();
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

  function renderChatMessages() {
    if (!chatMessagesEl) return;
    chatMessagesEl.innerHTML = "";
    if (!state.chatMessages.length) {
      const empty = document.createElement("div");
      empty.className = "chat-empty";
      if (!state.selectedChatSessionID) {
        empty.textContent = "Select a session";
      } else if ((getSelectedSession()?.session_type || "") !== "chat") {
        empty.textContent = "This session is currently in Terminal mode. Use \"Switch to Chat\" to continue the shared conversation here.";
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
      await fetchSessions();
      sidebar.closeSidebarOnMobile();
    });
  }

  function renderSessions() {
    renderSessionList({
      listEl: sessionsList,
      sessions: state.sessions,
      selectedSessionID: state.selectedChatSessionID,
      renderItem: (s, isSelected) => {
        const isSwitching = state.switchingSessionID === s.session_id;
        const li = document.createElement("li");
        li.classList.add("session-item");
        if (isSelected) li.classList.add("selected");
        const statusBadge = s.status === "running" ? "badge badge-running" : "badge";
        const canDelete = s.status !== "running" && !isSwitching;
        const modeLabel = s.session_type === "chat" ? "chat" : "pty";
        li.innerHTML = `
          <div class="session-main">
            <strong class="session-id">${escapeHtml(s.session_id.slice(0, 8))}</strong>
            <div class="session-badges">
              <span class="badge">${escapeHtml(modeLabel)}</span>
              <span class="${statusBadge}">${escapeHtml(s.status)}</span>
            </div>
          </div>
          <div class="session-sub">${escapeHtml(s.cwd || "-")}</div>
          <div class="session-detail"><span>active ${(s.active_instance_id || "-").slice(0, 8)}</span></div>
          ${s.exit_reason ? `<div class="session-detail"><span>reason ${escapeHtml(s.exit_reason)}</span></div>` : ""}
          <div class="session-actions">
            <button type="button" data-action="delete" class="btn-danger" ${canDelete ? "" : "disabled"}>Delete</button>
          </div>
        `;
        const deleteBtn = li.querySelector('[data-action="delete"]');
        deleteBtn.addEventListener("click", async (e) => {
          e.stopPropagation();
          await deleteSession(s);
        });
        li.addEventListener("click", () => attachChatSession(s.session_id));
        return li;
      },
    });
    workspace.render(getSelectedSession());
  }

  function renderInstanceHistory() {
    if (!chatInstanceHistoryList || !chatInstanceHistoryCount) return;
    chatInstanceHistoryList.innerHTML = "";
    const session = state.sessions.find((s) => s.session_id === state.selectedChatSessionID);
    const items = state.instanceHistory || [];
    chatInstanceHistoryCount.textContent = String(items.length);
    if (!state.selectedChatSessionID) {
      const li = document.createElement("li");
      li.className = "session-item";
      li.textContent = "Select a session";
      chatInstanceHistoryList.appendChild(li);
      return;
    }
    if (!items.length) {
      const li = document.createElement("li");
      li.className = "session-item";
      li.textContent = "No instances";
      chatInstanceHistoryList.appendChild(li);
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
      chatInstanceHistoryList.appendChild(li);
    }
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
    renderServers();
  }

  async function fetchSessions() {
    const q = state.selectedServerID ? `?server_id=${encodeURIComponent(state.selectedServerID)}` : "";
    const resp = await api(`/api/sessions${q}`);
    if (!resp.ok) return;
    const body = await resp.json();
    state.sessions = body.sessions || [];
    renderSessions();
    renderInstanceHistory();
    if (state.requestedSessionID && !state.chatMessages.length) {
      const target = state.sessions.find((s) => s.session_id === state.requestedSessionID);
      if (target) {
        state.requestedSessionID = "";
        await attachChatSession(target.session_id);
      }
    }
    if (state.selectedChatSessionID && !state.sessions.some((s) => s.session_id === state.selectedChatSessionID)) {
      state.selectedChatSessionID = "";
      state.instanceHistory = [];
      state.chatMessages = [];
      state.pendingScreenshots = [];
      renderInstanceHistory();
      renderChatMessages();
      renderPendingScreenshots();
      workspace.render(null);
    }
  }

  async function refreshAll() {
    await fetchServers();
    await fetchSessions();
  }

  async function deleteSession(session) {
    if (!session?.session_id) return;
    if (session.status === "running") {
      alert("cannot delete running session");
      return;
    }
    if (!window.confirm(`Delete session ${session.session_id.slice(0, 8)}?`)) return;
    const resp = await api(`/api/sessions/${encodeURIComponent(session.session_id)}`, { method: "DELETE" });
    if (!resp.ok) {
      alert(await resp.text());
      return;
    }
    if (state.selectedChatSessionID === session.session_id) {
      state.selectedChatSessionID = "";
      state.instanceHistory = [];
      state.chatMessages = [];
      state.pendingScreenshots = [];
      renderChatMessages();
      renderPendingScreenshots();
      renderInstanceHistory();
    }
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
    if (state.selectedChatSessionID !== sessionID) return;
    state.instanceHistory = (await resp.json()).instances || [];
    renderInstanceHistory();
  }

  async function switchSessionToTerminal(session) {
    if (!session?.session_id) return;
    const serverID = session.server_id || state.selectedServerID;
    if (!serverID) {
      alert("select a server first");
      return;
    }
    if (isWindowsServer(getServerByID(serverID))) {
      alert("PTY is not supported on Windows yet.");
      return;
    }
    if ((session.session_type || "pty") === "pty") {
      openTerminalView(session);
      return;
    }
    if (state.switchingSessionID) return;
    const sessionID = session.session_id;
    state.switchingSessionID = sessionID;
    renderSessions();
    try {
      const switchBody = {
        session_type: "pty",
        env: parseEnv(envInput ? envInput.value : ""),
        cols: 120,
        rows: 30,
      };
      const createResp = await api(`/api/sessions/${encodeURIComponent(sessionID)}/switch`, {
        method: "POST",
        body: JSON.stringify(switchBody),
      });
      if (!createResp.ok) {
        alert(formatSessionCreateError(await createResp.text()));
        return;
      }
      openTerminalView(session);
    } finally {
      state.switchingSessionID = "";
      renderSessions();
    }
  }

  async function switchSessionToChat(session) {
    if (!session?.session_id) return;
    if ((session.session_type || "pty") === "chat") {
      await attachChatSession(session.session_id);
      return;
    }
    if (state.switchingSessionID) return;
    const sessionID = session.session_id;
    state.switchingSessionID = sessionID;
    renderSessions();
    try {
      const createResp = await api(`/api/sessions/${encodeURIComponent(sessionID)}/switch`, {
        method: "POST",
        body: JSON.stringify({
          session_type: "chat",
          env: parseEnv(envInput ? envInput.value : ""),
        }),
      });
      if (!createResp.ok) {
        alert(formatSessionCreateError(await createResp.text()));
        return;
      }
      await fetchSessions();
      await attachChatSession(sessionID);
    } finally {
      state.switchingSessionID = "";
      renderSessions();
    }
  }

  function openTerminalView(session) {
    const serverID = session?.server_id || state.selectedServerID || "";
    const sessionID = session?.session_id || state.selectedChatSessionID || "";
    if (!sessionID) return;
    const query = new URLSearchParams();
    if (serverID) query.set("server_id", serverID);
    query.set("session_id", sessionID);
    window.location.href = `/?${query.toString()}`;
  }

  async function attachChatSession(sessionID) {
    if (!sessionID) return;
    state.selectedChatSessionID = sessionID;
    state.instanceHistory = [];
    state.chatMessages = [];
    state.pendingScreenshots = [];
    state.pendingTurns = 0;
    clearPendingSlowTimer();
    setRunState("", "");
    renderSessions();
    renderChatMessages();
    renderPendingScreenshots();
    renderInstanceHistory();
    workspace.render(getSelectedSession());
    fetchSessionInstances(sessionID);

    const session = getSelectedSession();
    if (!session || (session.session_type || "pty") !== "chat") {
      sidebar.closeSidebarOnMobile();
      return;
    }

    sendWS({ type: "attach", data: { session_id: sessionID, since_seq: 0 } });

    try {
      const resp = await api(`/api/sessions/${encodeURIComponent(sessionID)}/chat`);
      if (resp.ok) {
        const body = await resp.json();
        state.chatMessages = body.messages || [];
        renderChatMessages();
      }
    } catch (e) {
      console.error("[chat] failed to load history", e);
    }
    sidebar.closeSidebarOnMobile();
  }

  function sendChatMessage() {
    if (!chatInput) return;
    const text = chatInput.value.trim();
    if (!text && !state.pendingScreenshots.length) return;
    if (!state.selectedChatSessionID) {
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
      session_id: state.selectedChatSessionID,
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

  async function createChatSession() {
    const cwd = cwdInput?.value?.trim() || "";
    if (!cwd) {
      alert("cwd is required");
      return;
    }
    if (!state.selectedServerID) {
      alert("select a server first");
      return;
    }
    const body = {
      server_id: state.selectedServerID,
      session_type: "chat",
      session_id: (document.getElementById("sessionIdInput")?.value || "").trim().toLowerCase(),
      cwd,
      env: parseEnv(envInput ? envInput.value : ""),
    };
    if (!body.session_id) delete body.session_id;
    const resp = await api("/api/sessions", { method: "POST", body: JSON.stringify(body) });
    if (!resp.ok) {
      alert(formatSessionCreateError(await resp.text()));
      return;
    }
    const session = await resp.json();
    await fetchSessions();
    await attachChatSession(session.session_id);
  }

  tokenInput.value = state.token;
  saveTokenBtn?.addEventListener("click", () => applyUIToken(tokenInput.value.trim()));
  refreshServersBtn?.addEventListener("click", fetchServers);
  refreshSessionsBtn?.addEventListener("click", fetchSessions);
  newChatBtn?.addEventListener("click", createChatSession);
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

  sidebar.mount();
  renderChatMessages();
  renderPendingScreenshots();
  renderInstanceHistory();
  workspace.render(null);
  if (localStorage.getItem("ui_token")) {
    wsClient.connect();
    refreshAll();
  }
}
