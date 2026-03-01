import { escapeHtml } from "../shared/utils.js";
import { renderChatMarkdown } from "../chat/markdown.js";

/**
 * @param {object} ctx
 * @param {object} ctx.state
 * @param {function} ctx.sendWS
 * @param {function} ctx.getSelectedSession
 * @param {HTMLElement} ctx.chatMessagesEl
 * @param {HTMLElement} ctx.chatInput
 * @param {HTMLElement} ctx.chatRunState
 * @param {HTMLElement} ctx.chatAttachmentBar
 * @param {HTMLElement} ctx.chatAttachmentList
 */
export function createChatController(ctx) {
  const MAX_SCREENSHOTS_PER_MESSAGE = 3;
  const MAX_SCREENSHOT_BYTES = 900 * 1024;
  const MAX_SCREENSHOT_EDGE = 1800;
  const SLOW_RESPONSE_MS = 12000;

  function clearPendingSlowTimer() {
    if (!ctx.state.pendingSlowTimer) return;
    clearTimeout(ctx.state.pendingSlowTimer);
    ctx.state.pendingSlowTimer = null;
  }

  function setRunState(kind, text) {
    if (!ctx.chatRunState) return;
    const content = String(text || "").trim();
    if (!content) {
      ctx.chatRunState.hidden = true;
      ctx.chatRunState.className = "chat-run-state";
      ctx.chatRunState.textContent = "";
      return;
    }
    ctx.chatRunState.hidden = false;
    ctx.chatRunState.className = `chat-run-state ${kind || ""}`.trim();
    ctx.chatRunState.textContent = content;
  }

  function beginPendingTurn() {
    ctx.state.pendingTurns += 1;
    if (ctx.state.pendingTurns > 1) {
      setRunState("running", `Running... (${ctx.state.pendingTurns} requests pending)`);
      return;
    }
    setRunState("running", "Running... waiting for assistant response");
    clearPendingSlowTimer();
    ctx.state.pendingSlowTimer = setTimeout(() => {
      if (ctx.state.pendingTurns > 0) {
        setRunState("slow", "Still running... this is taking longer than usual");
      }
    }, SLOW_RESPONSE_MS);
  }

  function completePendingTurn() {
    if (ctx.state.pendingTurns <= 0) return;
    ctx.state.pendingTurns -= 1;
    if (ctx.state.pendingTurns > 0) {
      setRunState("running", `Running... (${ctx.state.pendingTurns} requests pending)`);
      return;
    }
    clearPendingSlowTimer();
    setRunState("", "");
  }

  function failPendingTurns(reason) {
    if (ctx.state.pendingTurns <= 0) return;
    ctx.state.pendingTurns = 0;
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
    const canvasCtx = canvas.getContext("2d");
    if (!canvasCtx) throw new Error("canvas_not_supported");
    canvasCtx.drawImage(img, 0, 0, width, height);

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
    if (!ctx.chatAttachmentBar || !ctx.chatAttachmentList) return;
    ctx.chatAttachmentList.innerHTML = "";
    if (!ctx.state.pendingScreenshots.length) {
      ctx.chatAttachmentBar.hidden = true;
      return;
    }
    ctx.chatAttachmentBar.hidden = false;
    for (let i = 0; i < ctx.state.pendingScreenshots.length; i += 1) {
      const entry = ctx.state.pendingScreenshots[i];
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
      removeBtn.textContent = "\u00d7";
      removeBtn.title = "Remove";
      removeBtn.addEventListener("click", () => {
        ctx.state.pendingScreenshots.splice(i, 1);
        renderPendingScreenshots();
      });
      item.appendChild(removeBtn);
      ctx.chatAttachmentList.appendChild(item);
    }
  }

  async function addScreenshotFiles(files) {
    const fileList = Array.isArray(files) ? files : Array.from(files || []);
    if (!fileList.length) return;

    const slots = MAX_SCREENSHOTS_PER_MESSAGE - ctx.state.pendingScreenshots.length;
    if (slots <= 0) {
      alert(`Only ${MAX_SCREENSHOTS_PER_MESSAGE} screenshots per message`);
      return;
    }

    const accepted = fileList.filter((f) => f && /^image\//i.test(f.type)).slice(0, slots);
    for (const file of accepted) {
      try {
        const dataURL = await optimizeScreenshot(file);
        ctx.state.pendingScreenshots.push({
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
    if (!ctx.chatMessagesEl) return;
    ctx.chatMessagesEl.innerHTML = "";
    if (!ctx.state.chatMessages.length) {
      const empty = document.createElement("div");
      empty.className = "chat-empty";
      if (!ctx.state.selectedSessionID) {
        empty.textContent = "Select a session";
      } else if ((ctx.getSelectedSession()?.session_type || "") !== "chat") {
        empty.textContent = 'This session is currently in Terminal mode. Use "Switch to Chat" to continue the shared conversation here.';
      } else {
        empty.textContent = "No messages yet";
      }
      ctx.chatMessagesEl.appendChild(empty);
      return;
    }

    for (const m of ctx.state.chatMessages) {
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
      meta.textContent = `${new Date(m.ts_ms).toLocaleTimeString()} \u2022 instance ${formatInstanceShort(m.instance_id)}`;
      bubble.appendChild(meta);
      ctx.chatMessagesEl.appendChild(bubble);
    }

    ctx.chatMessagesEl.scrollTop = ctx.chatMessagesEl.scrollHeight;
  }

  function sendChatMessage() {
    if (!ctx.chatInput) return;
    const text = ctx.chatInput.value.trim();
    if (!text && !ctx.state.pendingScreenshots.length) return;
    if (!ctx.state.selectedSessionID) {
      alert("Select or create a chat session first");
      return;
    }
    if ((ctx.getSelectedSession()?.session_type || "") !== "chat") {
      alert("This session is currently in Terminal mode. Switch to Chat first.");
      return;
    }
    const contentParts = [];
    if (text) {
      contentParts.push({ type: "text", text });
    }
    for (const entry of ctx.state.pendingScreenshots) {
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
    const sent = ctx.sendWS({
      type: "chat_in",
      session_id: ctx.state.selectedSessionID,
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
    ctx.chatInput.value = "";
    ctx.chatInput.style.height = "auto";
    ctx.state.pendingScreenshots = [];
    renderPendingScreenshots();
  }

  return {
    renderChatMessages,
    renderPendingScreenshots,
    addScreenshotFiles,
    sendChatMessage,
    beginPendingTurn,
    completePendingTurn,
    failPendingTurns,
    clearPendingSlowTimer,
    setRunState,
  };
}
