import { b64ToBytes, bytesToB64 } from "../shared/utils.js";

export function createTerminalController({
  onTermData,
  getSelectedSessionID,
  sendWS,
}) {
  let term = null;
  let fitAddon = null;
  const currentSessionLabel = document.getElementById("currentSessionLabel");
  const sessionHint = document.getElementById("sessionHint");

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

    term.onData((data) => {
      onTermData(data);
      sendWS({
        type: "term_in",
        session_id: getSelectedSessionID(),
        data_b64: bytesToB64(data),
      });
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
    currentSessionLabel.textContent = `Session: ${sessionID} (loading...)`;
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

  function markLoaded(sessionID) {
    currentSessionLabel.textContent = `Session: ${sessionID}`;
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
    sendResize,
    bindToolbarKeys,
    getTerm: () => term,
  };
}
