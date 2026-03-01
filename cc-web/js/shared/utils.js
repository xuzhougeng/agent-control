export const WINDOWS_PTY_UNSUPPORTED_ERROR = "PTY is not supported on Windows yet; use session_type=chat";

export function b64ToBytes(b64) {
  const bin = atob(b64);
  const arr = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i += 1) arr[i] = bin.charCodeAt(i);
  return arr;
}

export function bytesToB64(str) {
  const bytes = new TextEncoder().encode(str);
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin);
}

export function parseEnv(input) {
  const env = {};
  const pairs = (input || "").split(",");
  for (const pair of pairs) {
    const trimmed = pair.trim();
    if (!trimmed) continue;
    const idx = trimmed.indexOf("=");
    if (idx <= 0) continue;
    env[trimmed.slice(0, idx)] = trimmed.slice(idx + 1);
  }
  return env;
}

export function escapeHtml(str) {
  return String(str)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

export function decodeHtmlEntities(input) {
  const el = document.createElement("textarea");
  el.innerHTML = input;
  return el.value;
}

export function formatTime(ts) {
  const n = Number(ts);
  if (!Number.isFinite(n) || n <= 0) return String(ts || "-");
  try {
    return new Date(n).toLocaleString();
  } catch (_err) {
    return String(ts);
  }
}

export function debounce(fn, ms) {
  let timer;
  return function debounced(...args) {
    clearTimeout(timer);
    timer = setTimeout(() => fn.apply(this, args), ms);
  };
}

export function renderEmptyItem(list, text) {
  const li = document.createElement("li");
  li.className = "list-empty";
  li.textContent = text;
  list.appendChild(li);
}

export function normalizeValue(value, fallback = "-") {
  if (value === undefined || value === null || value === "") return fallback;
  return String(value);
}

export function downloadBlob(filename, text, mimeType) {
  const blob = new Blob([text], { type: mimeType });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

/**
 * Custom confirm dialog — returns a Promise<boolean>.
 * Falls back to window.confirm if the modal element is not in the DOM.
 */
export function customConfirm(message, { confirmLabel = "确认", cancelLabel = "取消", danger = false } = {}) {
  const modal = document.getElementById("customConfirmModal");
  if (!modal) return Promise.resolve(window.confirm(message));

  return new Promise((resolve) => {
    const msgEl = modal.querySelector(".cc-modal-message");
    const confirmBtn = modal.querySelector(".cc-modal-confirm");
    const cancelBtn = modal.querySelector(".cc-modal-cancel");

    if (msgEl) msgEl.textContent = message;
    if (confirmBtn) {
      confirmBtn.textContent = confirmLabel;
      confirmBtn.className = "cc-modal-confirm " + (danger ? "cc-modal-btn-danger" : "cc-modal-btn-primary");
    }
    if (cancelBtn) cancelBtn.textContent = cancelLabel;

    modal.removeAttribute("hidden");
    modal.setAttribute("aria-hidden", "false");
    if (confirmBtn) confirmBtn.focus();

    function cleanup(result) {
      modal.setAttribute("hidden", "");
      modal.setAttribute("aria-hidden", "true");
      confirmBtn.removeEventListener("click", onConfirm);
      cancelBtn.removeEventListener("click", onCancel);
      modal.removeEventListener("keydown", onKeydown);
      resolve(result);
    }

    function onConfirm() { cleanup(true); }
    function onCancel() { cleanup(false); }
    function onKeydown(e) {
      if (e.key === "Escape") cleanup(false);
    }

    confirmBtn.addEventListener("click", onConfirm);
    cancelBtn.addEventListener("click", onCancel);
    modal.addEventListener("keydown", onKeydown);
  });
}

export function timestampedFilename(prefix, ext) {
  const now = new Date();
  const pad = (n) => String(n).padStart(2, "0");
  const stamp = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`;
  return `${prefix}-${stamp}.${ext}`;
}
