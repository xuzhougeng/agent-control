import { createUIApi } from "../shared/http.js";

const BASE = "/api/registry";
const api = createUIApi(() => localStorage.getItem("ui_token") || "");

async function jsonFetch(method, path, body) {
  const res = await api(BASE + path, {
    method,
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ code: String(res.status) }));
    throw Object.assign(new Error(err.code || res.statusText), err);
  }
  if (res.status === 204) return null;
  return res.json();
}

export const skillsApi = {
  list:    (q)            => jsonFetch("GET", "/skills" + (q ? `?q=${encodeURIComponent(q)}` : "")),
  get:     (name, v)      => jsonFetch("GET", `/skills/${encodeURIComponent(name)}` + (v ? `?version=${v}` : "")),
  history: (name)         => jsonFetch("GET", `/skills/${encodeURIComponent(name)}/history`),
  install: (name, v, target) => jsonFetch("POST", "/install_request", { name, version: v || 0, target_agent_id: target }),
};
