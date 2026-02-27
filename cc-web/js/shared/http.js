function withAuth(init, token) {
  const headers = new Headers(init?.headers || {});
  headers.set("Authorization", `Bearer ${token || ""}`);
  if (init?.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  return { ...(init || {}), headers };
}

export function createUIApi(getToken) {
  return function api(path, init = {}) {
    return fetch(path, withAuth(init, getToken()));
  };
}

export function createAdminApi(getToken) {
  return function adminApi(path, init = {}) {
    return fetch(path, withAuth(init, getToken()));
  };
}

export function createTenantApi(getToken) {
  return function tenantApi(path, init = {}) {
    return fetch(path, withAuth(init, getToken()));
  };
}
