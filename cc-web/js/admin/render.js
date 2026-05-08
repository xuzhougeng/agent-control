import { escapeHtml, formatTime, normalizeValue, renderEmptyItem } from "../shared/utils.js";

export function makeAdminFieldRow(label, value, { mono = false } = {}) {
  const row = document.createElement("div");
  row.className = "admin-field-row";
  const key = document.createElement("div");
  key.className = "admin-field-key";
  key.textContent = label;
  const val = document.createElement("div");
  val.className = `admin-field-value${mono ? " mono" : ""}`;
  val.textContent = normalizeValue(value);
  row.appendChild(key);
  row.appendChild(val);
  return row;
}

export function renderAdminResult(container, data) {
  container.innerHTML = "";
  if (!data) {
    const empty = document.createElement("div");
    empty.className = "admin-result-empty";
    empty.textContent = "(no token generated)";
    container.appendChild(empty);
    return;
  }
  const card = document.createElement("div");
  card.className = "admin-result-card";
  const header = document.createElement("div");
  header.className = "admin-result-header";
  const title = document.createElement("strong");
  title.textContent = "Latest Tenant Token";
  header.appendChild(title);

  const fields = document.createElement("div");
  fields.className = "admin-field-list";
  fields.appendChild(makeAdminFieldRow("tenant", data.tenant_id, { mono: true }));
  fields.appendChild(makeAdminFieldRow("token", data.token, { mono: true }));
  fields.appendChild(makeAdminFieldRow("token_id", data.token_id, { mono: true }));
  fields.appendChild(makeAdminFieldRow("created", formatTime(data.created_at_ms)));

  card.appendChild(header);
  card.appendChild(fields);
  container.appendChild(card);
}

export function renderAdminImportResult(container, payload) {
  container.innerHTML = "";
  if (!payload) {
    const empty = document.createElement("div");
    empty.className = "admin-result-empty";
    empty.textContent = "(no import yet)";
    container.appendChild(empty);
    return;
  }

  const card = document.createElement("div");
  card.className = "admin-result-card";
  const header = document.createElement("div");
  header.className = "admin-result-header";
  const title = document.createElement("strong");
  title.textContent = "Import Summary";
  header.appendChild(title);
  card.appendChild(header);

  const fields = document.createElement("div");
  fields.className = "admin-field-list";
  fields.appendChild(makeAdminFieldRow("total", payload.total));
  fields.appendChild(makeAdminFieldRow("imported", payload.imported));
  fields.appendChild(makeAdminFieldRow("skipped", payload.skipped));
  fields.appendChild(makeAdminFieldRow("errors", payload.errors));
  card.appendChild(fields);
  container.appendChild(card);

  const issues = (payload.results || []).filter((item) => item && item.status !== "imported");
  if (!issues.length) return;

  const list = document.createElement("ul");
  list.className = "admin-import-list";
  for (const item of issues) {
    const li = document.createElement("li");
    const status = item.status || "error";
    li.className = `admin-import-item ${status}`;
    const reason = item.reason ? ` - ${item.reason}` : "";
    const tenant = item.tenant_id ? ` tenant=${item.tenant_id}` : "";
    const tokenID = item.token_id ? ` token_id=${item.token_id}` : "";
    const row = item.row || "-";
    li.textContent = `Row ${row}: ${status}${reason}${tenant}${tokenID}`;
    list.appendChild(li);
  }
  container.appendChild(list);
}

export function renderAdminTenantTokens(listEl, state, opts) {
  const { adminTokenSearch, updateAdminCopyState, onToggleSelect, onRevoke } = opts;
  listEl.innerHTML = "";
  const query = (adminTokenSearch?.value || "").toLowerCase().trim();
  const filtered = state.adminTenantTokens.filter((rec) => {
    if (!query) return true;
    return (rec.tenant_id || "").toLowerCase().includes(query)
      || (rec.token_id || "").toLowerCase().includes(query);
  });

  if (!filtered.length) {
    const li = document.createElement("li");
    li.className = "admin-token-item list-empty";
    li.textContent = "(no tenant tokens)";
    listEl.appendChild(li);
    return;
  }

  for (const rec of filtered) {
    const li = document.createElement("li");
    li.className = "admin-token-item";
    if (rec.token_id && rec.token_id === state.selectedAdminTokenID) {
      li.classList.add("selected");
    }

    li.innerHTML = `
      <div class="admin-token-head">
        <strong>${escapeHtml((rec.tenant_id || "(unknown)").slice(0, 8))}</strong>
        <span class="badge ${rec.revoked ? "badge-offline" : "badge-online"}">${rec.revoked ? "revoked" : "active"}</span>
      </div>
    `;

    const fields = document.createElement("div");
    fields.className = "admin-field-list";
    fields.appendChild(makeAdminFieldRow("tenant", rec.tenant_id, { mono: true }));
    fields.appendChild(makeAdminFieldRow("token_id", rec.token_id, { mono: true }));
    fields.appendChild(makeAdminFieldRow("created", formatTime(rec.created_at_ms)));
    li.appendChild(fields);

    if (rec.token_id) {
      li.tabIndex = 0;
      li.addEventListener("click", (event) => {
        if (event.target?.closest?.("button")) return;
        onToggleSelect(rec.token_id);
        updateAdminCopyState();
      });
      li.addEventListener("keydown", (event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onToggleSelect(rec.token_id);
          updateAdminCopyState();
        }
      });
    }

    if (!rec.revoked && rec.token_id) {
      const actions = document.createElement("div");
      actions.className = "admin-token-actions";
      const revokeBtn = document.createElement("button");
      revokeBtn.type = "button";
      revokeBtn.className = "btn-danger";
      revokeBtn.textContent = "Revoke";
      revokeBtn.addEventListener("click", () => onRevoke(rec.token_id));
      actions.appendChild(revokeBtn);
      li.appendChild(actions);
    }
    listEl.appendChild(li);
  }
}

export function renderAdminServers(list, rows, query) {
  list.innerHTML = "";
  const q = (query || "").toLowerCase().trim();
  const filtered = rows.filter((s) => {
    if (!q) return true;
    return (s.server_id || "").toLowerCase().includes(q)
      || (s.hostname || "").toLowerCase().includes(q)
      || (s.tenant_id || "").toLowerCase().includes(q)
      || (s.os || "").toLowerCase().includes(q)
      || (s.arch || "").toLowerCase().includes(q);
  });
  if (!filtered.length) {
    renderEmptyItem(list, "No servers");
    return;
  }

  for (const s of filtered) {
    const li = document.createElement("li");
    li.className = "server-item";
    const statusClass = s.status === "online" ? "badge-online" : "badge-offline";
    const lastSeen = s.last_seen_ms ? formatTime(s.last_seen_ms) : "-";
    const tags = Array.isArray(s.tags) ? s.tags : [];
    const isAgent = tags.includes("cc-agent") || s.claude_path === "cc-agent-builtin";
    const runtimeBadge = isAgent
      ? `<span class="badge badge-runtime-agent" title="cc-agent runtime">cc-agent</span>`
      : "";
    li.innerHTML = `
      <div class="server-main">
        <strong class="server-id">${escapeHtml(s.server_id)}</strong>
        ${runtimeBadge}
        <span class="badge ${statusClass}">${escapeHtml(s.status)}</span>
      </div>
      <div class="server-sub">
        <span class="server-host">${escapeHtml(s.hostname || "-")}</span>
        <span class="server-tags">${escapeHtml(s.os || "")}/${escapeHtml(s.arch || "")}</span>
        ${s.version ? `<span class="server-tags">v${escapeHtml(s.version)}</span>` : ""}
      </div>
      <div class="session-detail">
        <span>tenant: ${escapeHtml(s.tenant_id || "-")}</span>
        <span>seen: ${escapeHtml(lastSeen)}</span>
      </div>
    `;
    list.appendChild(li);
  }
}

export function renderAdminSessions(list, rows, query, statusFilter, onStop) {
  list.innerHTML = "";
  const q = (query || "").toLowerCase().trim();
  const filtered = rows.filter((s) => {
    if (statusFilter && s.status !== statusFilter) return false;
    if (!q) return true;
    return (s.session_id || "").toLowerCase().includes(q)
      || (s.server_id || "").toLowerCase().includes(q)
      || (s.tenant_id || "").toLowerCase().includes(q)
      || (s.cwd || "").toLowerCase().includes(q);
  });

  if (!filtered.length) {
    renderEmptyItem(list, "No sessions");
    return;
  }

  for (const s of filtered) {
    const li = document.createElement("li");
    li.className = "session-item";
    const statusBadge = s.status === "running" ? "badge badge-running" : "badge";
    li.innerHTML = `
      <div class="session-main">
        <strong class="session-id">${escapeHtml(s.session_id.slice(0, 8))}</strong>
        <div class="session-badges"><span class="${statusBadge}">${escapeHtml(s.status)}</span></div>
      </div>
      <div class="session-sub">${escapeHtml(s.cwd || "-")}</div>
      <div class="session-detail">
        <span>server: ${escapeHtml(s.server_id || "-")}</span>
        <span>tenant: ${escapeHtml(s.tenant_id || "-")}</span>
      </div>
      ${s.status === "running" ? `<div class="session-actions"><button type="button" class="btn-danger admin-stop-btn">Stop</button></div>` : ""}
    `;
    const stopBtn = li.querySelector(".admin-stop-btn");
    if (stopBtn) stopBtn.addEventListener("click", (e) => { e.stopPropagation(); onStop(s.session_id); });
    list.appendChild(li);
  }
}

export function renderAdminOverviewStats({ servers, sessions, tokens }) {
  const online = servers.filter((s) => s.status === "online").length;
  const offline = servers.length - online;
  const running = sessions.filter((s) => s.status === "running").length;
  const other = sessions.length - running;
  const active = tokens.filter((t) => !t.revoked).length;
  const revoked = tokens.filter((t) => t.revoked).length;

  const tenantIDs = new Set();
  for (const t of tokens) if (t.tenant_id) tenantIDs.add(t.tenant_id);
  for (const s of servers) if (s.tenant_id) tenantIDs.add(s.tenant_id);

  const setText = (id, text) => {
    const el = document.getElementById(id);
    if (el) el.textContent = text;
  };

  setText("statServersTotal", String(servers.length));
  setText("statServersOnline", `${online} online`);
  setText("statServersOffline", `${offline} offline`);
  setText("statSessionsTotal", String(sessions.length));
  setText("statSessionsRunning", `${running} running`);
  setText("statSessionsOther", `${other} other`);
  setText("statTokensTotal", String(tokens.length));
  setText("statTokensActive", `${active} active`);
  setText("statTokensRevoked", `${revoked} revoked`);
  setText("statTenantsTotal", String(tenantIDs.size));
  setText("statTenantsDetail", "unique tenant IDs");
}
