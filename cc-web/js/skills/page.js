import { skillsApi } from "./api.js";
import { renderList, renderDetail } from "./render.js";

export function initSkillsPage() {
  const listEl = document.getElementById("skills-list");
  const detailEl = document.getElementById("skills-detail");
  const searchEl = document.getElementById("skills-search");

  const state = { items: [], selected: null, history: [], detail: null, query: "" };

  async function reloadList() {
    state.items = await skillsApi.list(state.query);
    renderList(state.items, listEl, onSelect);
    if (state.selected && !state.items.find(i => i.name === state.selected)) {
      state.selected = null;
      state.detail = null;
      renderDetail(null, [], detailEl, onInstall);
    } else if (!state.selected && state.items.length > 0) {
      onSelect(state.items[0].name);
    }
  }

  async function onSelect(name) {
    state.selected = name;
    listEl.querySelectorAll("li").forEach(li => li.classList.toggle("selected", li.dataset.name === name));
    const [detail, history] = await Promise.all([
      skillsApi.get(name),
      skillsApi.history(name),
    ]);
    state.detail = detail;
    state.history = history;
    renderDetail(detail, history, detailEl, onInstall);
  }

  async function onInstall(skill) {
    const target = window.prompt("Install on which host? (server_id)");
    if (!target) return;
    try {
      await skillsApi.install(skill.name, skill.version, target);
      alert(`install request sent to ${target}`);
    } catch (err) {
      alert(`install failed: ${err.code || err.message}`);
    }
  }

  searchEl.addEventListener("input", debounce(() => {
    state.query = searchEl.value.trim();
    reloadList();
  }, 200));

  reloadList().catch(err => {
    detailEl.innerHTML = `<p class="placeholder">Failed to load: ${err.message}</p>`;
  });
}

function debounce(fn, ms) {
  let t = null;
  return (...args) => {
    clearTimeout(t);
    t = setTimeout(() => fn(...args), ms);
  };
}
