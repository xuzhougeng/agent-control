import { escapeHtml, renderEmptyItem } from "./utils.js";

// isCCAgent returns true when the server registered with the cc-agent runtime
// (the new in-process agent), as opposed to the cc-proxy PTY wrapper. Both
// signals are sent during /ws/agent register: cc-agent tags itself "cc-agent"
// and exposes claude_path="cc-agent-builtin".
function isCCAgent(server) {
  const tags = Array.isArray(server.tags) ? server.tags : [];
  if (tags.includes("cc-agent")) return true;
  return server.claude_path === "cc-agent-builtin";
}

export function renderServerList(listEl, servers, selectedServerID, onSelect) {
  listEl.innerHTML = "";
  if (!servers.length) {
    renderEmptyItem(listEl, "No servers");
    return;
  }
  for (const s of servers) {
    const li = document.createElement("li");
    li.classList.add("server-item");
    if (s.server_id === selectedServerID) li.classList.add("selected");
    const statusClass = s.status === "online" ? "badge-online" : "badge-offline";
    // Strip the runtime tag from the displayed list; it is shown as a badge.
    const otherTags = (s.tags || [])
      .filter((t) => t !== "cc-agent")
      .map(escapeHtml)
      .join(", ");
    const runtimeBadge = isCCAgent(s)
      ? `<span class="badge badge-runtime-agent" title="cc-agent runtime: native Go agent (vs cc-proxy PTY wrapper)">cc-agent</span>`
      : "";
    li.innerHTML = `
      <div class="server-main">
        <strong class="server-id">${escapeHtml(s.server_id)}</strong>
        ${runtimeBadge}
        <span class="badge ${statusClass}">${escapeHtml(s.status)}</span>
      </div>
      <div class="server-sub">
        <span class="server-host">${escapeHtml(s.hostname || "-")}</span>
        ${otherTags ? `<span class="server-tags">${otherTags}</span>` : ""}
      </div>
    `;
    li.addEventListener("click", () => onSelect(s));
    listEl.appendChild(li);
  }
}
