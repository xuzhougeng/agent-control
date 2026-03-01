import { createUIApi } from "../shared/http.js";
import { createSidebarController } from "../shared/sidebar.js";
import { createWSClient, setWSBadge } from "../shared/ws.js";
import { createTerminalController } from "../controller/terminal.js";
import { createWorkspaceShell } from "../shared/workspace-shell.js";
import { bytesToB64 } from "../shared/utils.js";
import { createChatController } from "./chat.js";
import { createSessionController } from "./session.js";
import { createWSHandler } from "./ws-handler.js";

export function initWorkspacePage() {
  const query = new URLSearchParams(window.location.search);

  // ── State ──
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

  // ── API ──
  const api = createUIApi(() => state.token);

  // ── DOM cache ──
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
  const chatPermissionBar = document.getElementById("chatPermissionBar");
  const chatPermissionToggle = document.getElementById("chatPermissionToggle");
  const chatPermissionBody = document.getElementById("chatPermissionBody");
  const permModeInput = document.getElementById("permModeInput");
  const permAllowedInput = document.getElementById("permAllowedInput");
  const permDisallowedInput = document.getElementById("permDisallowedInput");
  const permApplyBtn = document.getElementById("permApplyBtn");

  // ── Helpers ──
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

  // ── Terminal + Sidebar ──
  function sendWS(msg) {
    if (msg?.type === "term_in") {
      const chunk = wsHandler.decodeB64Text(msg.data_b64);
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

  // ── Chat controller ──
  const chat = createChatController({
    state,
    sendWS,
    getSelectedSession,
    chatMessagesEl,
    chatInput,
    chatRunState,
    chatAttachmentBar,
    chatAttachmentList,
    chatPermissionBar,
    chatPermissionBody,
    permModeInput,
    permAllowedInput,
    permDisallowedInput,
  });

  // ── Workspace shell ── (依赖 session.openView，延迟绑定)
  let session;
  const workspace = createWorkspaceShell({
    viewMode: state.currentView,
    getSelectedSession,
    onOpenTerminalView: (s) => session.openView("pty", s.session_id),
    onOpenChatView: (s) => session.openView("chat", s.session_id),
    onSwitchMode: (s) => session.switchSessionTo(state.currentView, s),
    onCopySession: async (s) => {
      try {
        await navigator.clipboard.writeText(s.session_id || "");
      } catch {
        alert("copy failed");
      }
    },
  });

  // ── Session controller ──
  session = createSessionController({
    state,
    api,
    terminal,
    workspace,
    sidebar,
    sendWS,
    syncQuery,
    updateViewVisibility,
    getSelectedSession,
    getServerByID,
    getSelectedServer,
    chat,
    serversList,
    sessionsList,
    approvalList,
    approvalCount,
    instanceHistoryList,
    instanceHistoryCount,
    cwdInput,
    sessionIDInput,
    envInput,
    permModeInput,
    permAllowedInput,
    permDisallowedInput,
  });

  // ── WS handler ──
  const wsHandler = createWSHandler({
    state,
    e2eState,
    terminal,
    chat,
    session,
  });

  // ── WS client ──
  const wsClient = createWSClient({
    getToken: () => state.token,
    shouldConnect: () => true,
    setStatus: setWSBadge,
    onOpen: ({ send }) => {
      state.ws = wsClient.getSocket();
      session.attachSelectedView(send);
    },
    onMessage: (msg) => wsHandler.handleWS(msg),
    onVersionError: (data) => {
      const message = data?.message || "Protocol version mismatch";
      setWSBadge(false);
      alert(`Connection error: ${message}\nPlease refresh the page to get the latest client.`);
    },
    onError: (e) => console.error("[ws] error", e),
  });
  wsHandler.setWSClient(wsClient);

  // ── Token ──
  function applyUIToken(token) {
    state.token = token || "";
    tokenInput.value = state.token;
    localStorage.setItem("ui_token", state.token);
    wsClient.reconnect();
    session.refreshAll();
  }

  // ── Init ──
  terminal.init();
  terminal.bindToolbarKeys();
  sidebar.mount();
  window.__CC_E2E__ = {
    sendTerminalInput,
    getTerminalIOState: () => ({ ...e2eState }),
  };
  updateViewVisibility();
  session.renderInstanceHistory();
  session.renderApprovals();
  chat.renderChatMessages();
  chat.renderPendingScreenshots();
  workspace.render(null);

  // ── Event bindings ──
  tokenInput.value = state.token;
  saveTokenBtn?.addEventListener("click", () => applyUIToken(tokenInput.value.trim()));

  const pageHeaderEl = document.querySelector("header");
  const headerCollapseBtn = document.getElementById("headerCollapseBtn");
  if (localStorage.getItem("header_collapsed") === "1") {
    pageHeaderEl?.classList.add("collapsed");
  }
  headerCollapseBtn?.addEventListener("click", () => {
    const isCollapsed = pageHeaderEl?.classList.toggle("collapsed");
    localStorage.setItem("header_collapsed", isCollapsed ? "1" : "0");
  });
  document.getElementById("refreshServersBtn")?.addEventListener("click", session.fetchServers);
  document.getElementById("refreshSessionsBtn")?.addEventListener("click", session.fetchSessions);
  newSessionBtn?.addEventListener("click", session.createNewSession);
  chatSendBtn?.addEventListener("click", chat.sendChatMessage);
  chatPermissionToggle?.addEventListener("click", () => {
    const body = chatPermissionBody;
    if (!body) return;
    body.hidden = !body.hidden;
    if (chatPermissionToggle) chatPermissionToggle.textContent = body.hidden ? "▼" : "▲";
  });
  permApplyBtn?.addEventListener("click", () => session.applyPermissions());
  chatAttachBtn?.addEventListener("click", () => chatImageInput?.click());
  chatImageInput?.addEventListener("change", async () => {
    await chat.addScreenshotFiles(chatImageInput.files);
    chatImageInput.value = "";
  });

  if (chatInput) {
    chatInput.addEventListener("keydown", (e) => {
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        chat.sendChatMessage();
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
      await chat.addScreenshotFiles(imageFiles);
    });
  }

  if (localStorage.getItem("ui_token")) {
    wsClient.connect();
    session.refreshAll();
  }
}
