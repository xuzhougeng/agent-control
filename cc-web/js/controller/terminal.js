import { b64ToBytes, bytesToB64 } from "../shared/utils.js";

export function createTerminalController({
  onTermData,
  getSelectedSessionID,
  sendWS,
}) {
  const FOCUS_REPORT_SEQUENCES = new Set(["\x1b[I", "\x1b[O"]);
  const platform = String(window.navigator?.userAgentData?.platform || window.navigator?.platform || "").toLowerCase();
  const isApplePlatform = platform.includes("mac") || platform.includes("iphone") || platform.includes("ipad") || platform.includes("ipod");
  let term = null;
  let fitAddon = null;
  const currentSessionLabel = document.getElementById("currentSessionLabel");
  const sessionHint = document.getElementById("sessionHint");
  const terminalHost = document.getElementById("terminal");

  function getTerminalSelectionText() {
    if (term && typeof term.getSelection === "function") {
      const selected = String(term.getSelection() || "");
      if (selected) return selected;
    }
    const globalSelection = window.getSelection?.();
    return String(globalSelection?.toString() || "");
  }

  async function writeClipboardText(text) {
    const content = String(text || "");
    if (!content) return false;
    if (window.navigator?.clipboard?.writeText) {
      try {
        await window.navigator.clipboard.writeText(content);
        return true;
      } catch {}
    }

    const scratch = document.createElement("textarea");
    scratch.value = content;
    scratch.setAttribute("readonly", "");
    scratch.style.position = "fixed";
    scratch.style.top = "-1000px";
    scratch.style.left = "-1000px";
    scratch.style.opacity = "0";
    document.body.appendChild(scratch);
    scratch.focus();
    scratch.select();
    let copied = false;
    try {
      copied = Boolean(document.execCommand("copy"));
    } catch {}
    scratch.remove();
    if (term && typeof term.focus === "function") term.focus();
    return copied;
  }

  function isCopyShortcut(event) {
    const key = String(event?.key || "").toLowerCase();
    if (key !== "c") return false;
    if (isApplePlatform) {
      return event.metaKey && !event.ctrlKey && !event.altKey;
    }
    return event.ctrlKey && event.shiftKey && !event.altKey;
  }

  function isCtrlCWithSelectionShortcut(event, hasSelection) {
    if (!hasSelection || isApplePlatform) return false;
    const key = String(event?.key || "").toLowerCase();
    return key === "c" && event.ctrlKey && !event.shiftKey && !event.altKey && !event.metaKey;
  }

  function forwardInput(data) {
    onTermData(data);
    sendWS({
      type: "term_in",
      session_id: getSelectedSessionID(),
      data_b64: bytesToB64(data),
    });
  }

  function formatSessionLabel(sessionID, instanceID, suffix = "") {
    const instanceText = instanceID ? ` • Instance: ${instanceID}` : "";
    const suffixText = suffix ? ` ${suffix}` : "";
    return `Session: ${sessionID}${instanceText}${suffixText}`;
  }

  function init() {
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
    term.open(terminalHost);
    fitAddon.fit();

    if (typeof term.attachCustomKeyEventHandler === "function") {
      term.attachCustomKeyEventHandler((event) => {
        if (event.type !== "keydown") return true;
        const selected = getTerminalSelectionText();
        if (!isCopyShortcut(event) && !isCtrlCWithSelectionShortcut(event, Boolean(selected))) return true;
        event.preventDefault();
        event.stopPropagation();
        if (selected) {
          void writeClipboardText(selected);
        }
        return false;
      });
    }

    if (terminalHost) {
      terminalHost.addEventListener("copy", (event) => {
        const selected = getTerminalSelectionText();
        if (!selected) return;
        if (event.clipboardData?.setData) {
          event.preventDefault();
          event.clipboardData.setData("text/plain", selected);
          return;
        }
        event.preventDefault();
        void writeClipboardText(selected);
      });
    }

    window.addEventListener("resize", () => {
      fitAddon.fit();
      sendResize();
    });
    if (terminalHost) {
      new ResizeObserver(() => {
        fitAddon.fit();
        sendResize();
      }).observe(terminalHost);
    }

    term.onData((data) => {
      if (FOCUS_REPORT_SEQUENCES.has(data)) {
        return;
      }
      forwardInput(data);
    });
  }

  function syncLayout() {
    if (!fitAddon) return;
    fitAddon.fit();
    sendResize();
  }

  function writeOutput(msg, pendingFirstOutputSessionID, onLoaded) {
    if (!term || !msg?.data_b64) return;
    if (pendingFirstOutputSessionID === msg.session_id) {
      term.write(b64ToBytes(msg.data_b64), () => {
        term.scrollToBottom();
        onLoaded();
      });
    } else {
      term.write(b64ToBytes(msg.data_b64));
    }
  }

  function resetForSession(sessionID) {
    if (!term) return;
    currentSessionLabel.textContent = formatSessionLabel(sessionID, "", "(loading...)");
    if (sessionHint) sessionHint.hidden = false;
    term.reset();
    term.scrollToBottom();
  }

  function clearSession() {
    if (!term) return;
    currentSessionLabel.textContent = "Session: (none)";
    if (sessionHint) sessionHint.hidden = true;
    term.reset();
    term.scrollToBottom();
  }

  function markLoaded(sessionID, instanceID = "") {
    currentSessionLabel.textContent = formatSessionLabel(sessionID, instanceID);
  }

  function setSessionInfo(sessionID, instanceID = "", loading = false) {
    if (!currentSessionLabel) return;
    if (!sessionID) {
      currentSessionLabel.textContent = "Session: (none)";
      return;
    }
    currentSessionLabel.textContent = formatSessionLabel(sessionID, instanceID, loading ? "(loading...)" : "");
  }

  function sendQuickKey(keyValue) {
    const sessionID = getSelectedSessionID();
    if (!sessionID) {
      alert("No session attached — click a session first");
      return;
    }
    sendWS({ type: "term_in", session_id: sessionID, data_b64: bytesToB64(keyValue) });
  }

  function sendResize() {
    const sessionID = getSelectedSessionID();
    if (!sessionID || !term) return;
    sendWS({ type: "resize", session_id: sessionID, data: { cols: term.cols, rows: term.rows } });
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
      if (event.button !== 0) return;
      event.preventDefault();
      if (button.setPointerCapture) button.setPointerCapture(event.pointerId);
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
    button.addEventListener("click", () => {
      if (Date.now() < suppressClickUntil) return;
      step();
    });
  }

  function bindToolbarKeys() {
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

  return {
    init,
    syncLayout,
    writeOutput,
    resetForSession,
    clearSession,
    markLoaded,
    setSessionInfo,
    sendResize,
    bindToolbarKeys,
    getTerm: () => term,
  };
}
