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
  const switchBtn = document.getElementById("workspaceSwitchModeBtn");
  const copyBtn = document.getElementById("workspaceCopyBtn");

  if (viewBadge) {
    viewBadge.textContent = `${modeLabel(viewMode)} View`;
  }
  if (terminalBtn && viewMode === "pty") {
    terminalBtn.classList.add("active");
  }
  if (chatBtn && viewMode === "chat") {
    chatBtn.classList.add("active");
  }

  terminalBtn?.addEventListener("click", () => {
    const session = getSelectedSession();
    if (session) onOpenTerminalView(session);
  });
  chatBtn?.addEventListener("click", () => {
    const session = getSelectedSession();
    if (session) onOpenChatView(session);
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
      if (hint) hint.textContent = "Select a unified session from the left to inspect or switch modes.";
      if (serverText) serverText.textContent = "-";
      if (cwdText) cwdText.textContent = "-";
      if (instanceText) instanceText.textContent = "-";
      if (createdText) createdText.textContent = "-";
      if (terminalBtn) terminalBtn.disabled = true;
      if (chatBtn) chatBtn.disabled = true;
      if (switchBtn) {
        switchBtn.disabled = true;
        switchBtn.textContent = `Switch to ${modeLabel(viewMode)}`;
      }
      if (copyBtn) copyBtn.disabled = true;
      return;
    }

    const activeMode = String(session.session_type || "").trim() || "pty";
    const matchesView = activeMode === viewMode;
    const targetLabel = modeLabel(viewMode);
    const createdAt = session.created_at_ms ? new Date(session.created_at_ms).toLocaleString() : "-";

    if (modeBadge) modeBadge.textContent = `Mode: ${modeLabel(activeMode)}`;
    if (statusBadge) {
      statusBadge.className = statusBadgeClass(session.status);
      statusBadge.textContent = session.status || "-";
    }
    if (title) title.textContent = session.session_id || "(unknown)";
    if (hint) {
      hint.textContent = matchesView
        ? `This unified session is currently active in ${targetLabel} mode.`
        : `This unified session is currently active in ${modeLabel(activeMode)} mode. Use "Switch to ${targetLabel}" to move the Claude conversation into this view.`;
    }
    if (serverText) serverText.textContent = session.server_id || "-";
    if (cwdText) cwdText.textContent = session.cwd || "-";
    if (instanceText) instanceText.textContent = session.active_instance_id || "-";
    if (createdText) createdText.textContent = createdAt;

    if (terminalBtn) terminalBtn.disabled = false;
    if (chatBtn) chatBtn.disabled = false;
    if (switchBtn) {
      switchBtn.disabled = matchesView;
      switchBtn.textContent = matchesView ? `${targetLabel} Active` : `Switch to ${targetLabel}`;
    }
    if (copyBtn) copyBtn.disabled = false;
  }

  return { render };
}
