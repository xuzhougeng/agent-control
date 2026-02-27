import { renderEmptyItem } from "./utils.js";

export function renderSessionList({
  listEl,
  sessions,
  wantType,
  selectedSessionID,
  emptyText = "No sessions",
  renderItem,
}) {
  listEl.innerHTML = "";
  const filtered = sessions.filter((s) => (s.session_type || "pty") === wantType);
  if (!filtered.length) {
    renderEmptyItem(listEl, emptyText);
    return;
  }
  for (const session of filtered) {
    const li = renderItem(session, session.session_id === selectedSessionID);
    if (li) listEl.appendChild(li);
  }
}
