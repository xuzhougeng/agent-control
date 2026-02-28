function modeLabel(mode) {
  return mode === "pty" ? "Terminal" : "Chat";
}

function statusBadgeClass(status) {
  if (status === "running") return "badge badge-running";
  if (status === "error") return "badge badge-pending";
  return "badge";
}

export function createWorkspaceShell({
  viewMode,
  getSelectedSession,
  onOpenTerminalView,
  onOpenChatView,
  onSwitchMode,
  onCopySession,
}) {
  const viewBadge = document.getElementById("workspaceViewBadge");
  const modeBadge = document.getElementById("workspaceModeBadge");
  const statusBadge = document.getElementById("workspaceStatusBadge");
  const title = document.getElementById("workspaceSessionTitle");
  const hint = document.getElementById("workspaceHint");
  const serverText = document.getElementById("workspaceServerText");
  const cwdText = document.getElementById("workspaceCwdText");
  const instanceText = document.getElementById("workspaceInstanceText");
  const createdText = document.getElementById("workspaceCreatedText");
  const terminalBtn = document.getElementById("workspaceOpenTerminalBtn");
  const chatBtn = document.getElementById("workspaceOpenChatBtn");
  const toggleViewBtn = document.getElementById("workspaceToggleViewBtn");
  const switchBtn = document.getElementById("workspaceSwitchModeBtn");
  const copyBtn = document.getElementById("workspaceCopyBtn");
  const mobileMedia = window.matchMedia("(max-width: 900px)");

  let currentViewMode = viewMode;

  function applyViewMode() {
    if (viewBadge) {
      viewBadge.textContent = `${modeLabel(currentViewMode)} View`;
    }
    if (toggleViewBtn) {
      const nextMode = currentViewMode === "pty" ? "chat" : "pty";
      toggleViewBtn.textContent = modeLabel(nextMode);
      toggleViewBtn.setAttribute("aria-label", `Open ${modeLabel(nextMode)} view`);
    }
    if (terminalBtn) terminalBtn.classList.toggle("active", currentViewMode === "pty");
    if (chatBtn) chatBtn.classList.toggle("active", currentViewMode === "chat");
  }

  applyViewMode();

  terminalBtn?.addEventListener("click", () => {
    const session = getSelectedSession();
    if (session) onOpenTerminalView(session);
  });
  chatBtn?.addEventListener("click", () => {
    const session = getSelectedSession();
    if (session) onOpenChatView(session);
  });
  toggleViewBtn?.addEventListener("click", () => {
    const session = getSelectedSession();
    if (!session) return;
    if (currentViewMode === "pty") {
      onOpenChatView(session);
      return;
    }
    onOpenTerminalView(session);
  });
  switchBtn?.addEventListener("click", () => {
    const session = getSelectedSession();
    if (session) onSwitchMode(session);
  });
  copyBtn?.addEventListener("click", () => {
    const session = getSelectedSession();
    if (session) onCopySession(session);
  });

  function render(session) {
    if (!session) {
      if (modeBadge) modeBadge.textContent = "Mode: -";
      if (statusBadge) {
        statusBadge.className = "badge";
        statusBadge.textContent = "No Session";
      }
      if (title) title.textContent = "No session selected";
      if (hint) hint.textContent = "Pick a session from the left.";
      if (serverText) serverText.textContent = "-";
      if (cwdText) cwdText.textContent = "-";
      if (instanceText) instanceText.textContent = "-";
      if (createdText) createdText.textContent = "-";
      if (terminalBtn) terminalBtn.disabled = true;
      if (chatBtn) chatBtn.disabled = true;
      if (toggleViewBtn) toggleViewBtn.disabled = true;
      if (switchBtn) {
        switchBtn.disabled = true;
        switchBtn.textContent = mobileMedia.matches ? "Switch" : `Switch to ${modeLabel(currentViewMode)}`;
      }
      if (copyBtn) copyBtn.disabled = true;
      return;
    }

    const activeMode = String(session.session_type || "").trim() || "pty";
    const matchesView = activeMode === currentViewMode;
    const targetLabel = modeLabel(currentViewMode);
    const createdAt = session.created_at_ms ? new Date(session.created_at_ms).toLocaleString() : "-";

    if (modeBadge) modeBadge.textContent = `Mode: ${modeLabel(activeMode)}`;
    if (statusBadge) {
      statusBadge.className = statusBadgeClass(session.status);
      statusBadge.textContent = session.status || "-";
    }
    if (title) title.textContent = session.session_id || "(unknown)";
    if (hint) {
      hint.textContent = matchesView
        ? `Continue in ${targetLabel}.`
        : `Active in ${modeLabel(activeMode)}. Switch to ${targetLabel} to continue here.`;
    }
    if (serverText) serverText.textContent = session.server_id || "-";
    if (cwdText) cwdText.textContent = session.cwd || "-";
    if (instanceText) instanceText.textContent = session.active_instance_id || "-";
    if (createdText) createdText.textContent = createdAt;

    if (terminalBtn) terminalBtn.disabled = false;
    if (chatBtn) chatBtn.disabled = false;
    if (toggleViewBtn) toggleViewBtn.disabled = false;
    if (switchBtn) {
      switchBtn.disabled = matchesView;
      if (mobileMedia.matches) {
        switchBtn.textContent = matchesView ? "Active" : "Switch";
      } else {
        switchBtn.textContent = matchesView ? `${targetLabel} Active` : `Switch to ${targetLabel}`;
      }
    }
    if (copyBtn) copyBtn.disabled = false;
  }

  function setViewMode(nextViewMode) {
    currentViewMode = nextViewMode === "chat" ? "chat" : "pty";
    applyViewMode();
  }

  mobileMedia.addEventListener("change", () => {
    applyViewMode();
    render(getSelectedSession());
  });

  return { render, setViewMode };
}
