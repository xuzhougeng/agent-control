import { escapeHtml, renderEmptyItem } from "./utils.js";

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
    const tags = (s.tags || []).map(escapeHtml).join(", ");
    li.innerHTML = `
      <div class="server-main">
        <strong class="server-id">${escapeHtml(s.server_id)}</strong>
        <span class="badge ${statusClass}">${escapeHtml(s.status)}</span>
      </div>
      <div class="server-sub">
        <span class="server-host">${escapeHtml(s.hostname || "-")}</span>
        ${tags ? `<span class="server-tags">${tags}</span>` : ""}
      </div>
    `;
    li.addEventListener("click", () => onSelect(s));
    listEl.appendChild(li);
  }
}
