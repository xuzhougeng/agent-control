import { createUIApi } from "../shared/http.js";
import { createSidebarController } from "../shared/sidebar.js";
import { createWSClient, setWSBadge } from "../shared/ws.js";
import { escapeHtml, parseEnv } from "../shared/utils.js";
import { renderServerList } from "../shared/server-list.js";
import { renderSessionList } from "../shared/session-list.js";
import { renderChatMarkdown } from "./markdown.js";

export function initChatPage() {
  const state = {
    token: localStorage.getItem("ui_token") || "admin-dev-token",
    ws: null,
    servers: [],
    sessions: [],
    selectedServerID: new URLSearchParams(window.location.search).get("server_id") || "",
    selectedChatSessionID: "",
    chatMessages: [],
    chatWorkerSessionID: "",
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
  const chatSessionInfo = document.getElementById("chatSessionInfo");
  const chatSessionIdText = document.getElementById("chatSessionIdText");
  const chatCopySessionBtn = document.getElementById("chatCopySessionBtn");

  const sidebar = createSidebarController({
    isControllerPage: false,
    isChatPage: true,
    onLayoutChange: () => {},
    approvalDetails: document.getElementById("approvalDetails"),
  });

  const wsClient = createWSClient({
    getToken: () => state.token,
    shouldConnect: () => true,
    setStatus: setWSBadge,
    onOpen: ({ send }) => {
      state.ws = wsClient.getSocket();
      if (state.selectedChatSessionID) {
        send({ type: "attach", data: { session_id: state.selectedChatSessionID, since_seq: 0 } });
      }
    },
    onMessage: (msg, { send: _send }) => {
      if (msg.type === "chat_msg" && msg.data) {
        const cm = msg.data;
        if (cm.session_id === state.selectedChatSessionID) {
          state.chatMessages.push(cm);
          renderChatMessages();
        }
        return;
      }
      if (msg.type === "session_update" && msg.data) {
        const data = msg.data;
        if (data.session_id === state.selectedChatSessionID && data.worker_session_id) {
          state.chatWorkerSessionID = data.worker_session_id;
          updateChatSessionInfo();
        }
        fetchSessions();
      }
    },
    onError: (e) => console.error("[ws] error", e),
  });

  function sendWS(msg) {
    return wsClient.send(msg);
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

  function renderChatMessages() {
    if (!chatMessagesEl) return;
    chatMessagesEl.innerHTML = "";
    if (!state.chatMessages.length) {
      const empty = document.createElement("div");
      empty.className = "chat-empty";
      empty.textContent = "No messages yet";
      chatMessagesEl.appendChild(empty);
      return;
    }

    for (const m of state.chatMessages) {
      const bubble = document.createElement("div");
      bubble.className = `chat-bubble ${m.role === "user" ? "user" : "assistant"}`;
      const body = document.createElement("div");
      body.className = "chat-markdown";
      body.innerHTML = renderChatMarkdown(m.content);
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
      meta.textContent = new Date(m.ts_ms).toLocaleTimeString();
      bubble.appendChild(meta);
      chatMessagesEl.appendChild(bubble);
    }

    chatMessagesEl.scrollTop = chatMessagesEl.scrollHeight;
  }

  function updateChatSessionInfo() {
    if (!chatSessionInfo) return;
    if (state.chatWorkerSessionID) {
      chatSessionIdText.textContent = `Session: ${state.chatWorkerSessionID}`;
      chatSessionInfo.hidden = false;
    } else {
      chatSessionInfo.hidden = true;
      chatSessionIdText.textContent = "";
    }
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
      wantType: "chat",
      selectedSessionID: state.selectedChatSessionID,
      renderItem: (s, isSelected) => {
      const li = document.createElement("li");
      li.classList.add("session-item");
      if (isSelected) li.classList.add("selected");
      const statusBadge = s.status === "running" ? "badge badge-running" : "badge";
      const canDelete = s.status !== "running";
      li.innerHTML = `
        <div class="session-main">
          <strong class="session-id">${escapeHtml(s.session_id.slice(0, 8))}</strong>
          <span class="${statusBadge}">${escapeHtml(s.status)}</span>
        </div>
        <div class="session-sub">${escapeHtml(s.cwd || "-")}</div>
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
      state.chatMessages = [];
      state.chatWorkerSessionID = "";
      renderChatMessages();
      updateChatSessionInfo();
    }
    await fetchSessions();
  }

  async function attachChatSession(sessionID) {
    if (!sessionID) return;
    state.selectedChatSessionID = sessionID;
    state.chatMessages = [];
    state.chatWorkerSessionID = "";
    renderSessions();
    renderChatMessages();
    updateChatSessionInfo();

    const sess = state.sessions.find((s) => s.session_id === sessionID);
    if (sess?.worker_session_id) {
      state.chatWorkerSessionID = sess.worker_session_id;
      updateChatSessionInfo();
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
    const content = chatInput.value.trim();
    if (!content) return;
    if (!state.selectedChatSessionID) {
      alert("Select or create a chat session first");
      return;
    }
    sendWS({ type: "chat_in", session_id: state.selectedChatSessionID, data: { content } });
    chatInput.value = "";
    chatInput.style.height = "auto";
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
      cwd,
      env: parseEnv(envInput ? envInput.value : ""),
    };
    const resp = await api("/api/sessions", { method: "POST", body: JSON.stringify(body) });
    if (!resp.ok) {
      alert(await resp.text());
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
  }

  chatCopySessionBtn?.addEventListener("click", async () => {
    if (!state.chatWorkerSessionID) return;
    try {
      await navigator.clipboard.writeText(state.chatWorkerSessionID);
      chatCopySessionBtn.textContent = "Copied!";
      setTimeout(() => {
        chatCopySessionBtn.textContent = "Copy";
      }, 1500);
    } catch (_err) {
      alert("Copy failed");
    }
  });

  sidebar.mount();
  renderChatMessages();
  if (localStorage.getItem("ui_token")) {
    wsClient.connect();
    refreshAll();
  }
}
