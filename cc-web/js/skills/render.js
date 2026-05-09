export function renderList(items, container, onSelect) {
  container.innerHTML = "";
  if (!items || items.length === 0) {
    container.innerHTML = `<li class="placeholder">No skills.</li>`;
    return;
  }
  for (const item of items) {
    const li = document.createElement("li");
    li.dataset.name = item.name;
    li.innerHTML = `
      <div class="skills-row-name">${escapeHtml(item.name)}</div>
      <div class="skills-row-meta">v${item.version} · ${escapeHtml(item.author_server_id)} · ${formatAge(item.created_at_unix)}</div>
      <div class="skills-row-desc">${escapeHtml(item.description || "")}</div>
    `;
    li.addEventListener("click", () => onSelect(item.name));
    container.appendChild(li);
  }
}

export function renderDetail(skill, history, container, onInstall) {
  if (!skill) {
    container.innerHTML = `<p class="placeholder">Select a skill on the left.</p>`;
    return;
  }
  container.innerHTML = `
    <div class="skills-detail-head">
      <h3>${escapeHtml(skill.name)} <small>v${skill.version}</small></h3>
      <div class="muted">by ${escapeHtml(skill.author_server_id)} · ${formatAge(skill.created_at_unix)}</div>
    </div>
    <h4>Prompt</h4>
    <pre class="skills-prompt">${escapeHtml(skill.prompt)}</pre>
    <h4>Tools</h4>
    <div class="skills-tools">${(skill.tools || []).map(t => `<span class="tag">${escapeHtml(t)}</span>`).join("")}</div>
    ${(skill.examples && skill.examples.length) ? `
      <h4>Examples</h4>
      <ul>${skill.examples.map(e => `<li>${escapeHtml(e)}</li>`).join("")}</ul>` : ""}
    <h4>History</h4>
    <table class="skills-history">
      <tr><th>Version</th><th>Author</th><th>When</th><th>Description</th></tr>
      ${(history || []).map(h => `
        <tr><td>v${h.version}</td><td>${escapeHtml(h.author_server_id)}</td>
            <td>${formatAge(h.created_at_unix)}</td><td>${escapeHtml(h.description || "")}</td></tr>
      `).join("")}
    </table>
    <button id="skills-install-btn" class="btn-primary">Install on host…</button>
  `;
  container.querySelector("#skills-install-btn").addEventListener("click", () => onInstall(skill));
}

function escapeHtml(s) {
  return String(s ?? "").replace(/[&<>"']/g, c => ({"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;","'":"&#39;"}[c]));
}

function formatAge(unix) {
  if (!unix) return "";
  const sec = Math.max(0, Math.floor(Date.now() / 1000) - unix);
  if (sec < 60) return `${sec}s ago`;
  if (sec < 3600) return `${Math.floor(sec / 60)}m ago`;
  if (sec < 86400) return `${Math.floor(sec / 3600)}h ago`;
  return `${Math.floor(sec / 86400)}d ago`;
}
