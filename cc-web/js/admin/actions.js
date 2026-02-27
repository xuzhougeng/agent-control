import {
  buildAdminImportRecords,
  cacheAdminToken,
  exportAdminTokensCsv,
  exportAdminTokensJson,
  formatTenantTokenClipboard,
  getCachedAdminToken,
} from "./tokens.js";
import {
  renderAdminImportResult,
  renderAdminOverviewStats,
  renderAdminResult,
  renderAdminServers,
  renderAdminSessions,
} from "./render.js";

export function createAdminActions(ctx) {
  const {
    state,
    adminApi,
    adminTokenInput,
    adminTenantList,
    adminImportResult,
    adminResult,
    adminServersSearch,
    adminSessionsSearch,
    adminSessionsStatusFilter,
    setAdminMessage,
    setAdminImportBusy,
    renderTokensList,
    updateAdminCopyState,
  } = ctx;

  async function createAdminToken() {
    if (!state.adminVerified) return setAdminMessage("Verify admin token first.", true);
    const token = adminTokenInput.value.trim();
    if (!token) return setAdminMessage("Admin token is required.", true);

    state.adminToken = token;
    localStorage.setItem("admin_token", state.adminToken);
    setAdminMessage("Generating tenant token...");

    let resp;
    try {
      resp = await adminApi("/admin/tokens", { method: "POST", body: JSON.stringify({ type: "tenant" }) });
    } catch (err) {
      setAdminMessage(`Request failed: ${String(err)}`, true);
      return;
    }
    if (!resp.ok) return setAdminMessage(await resp.text(), true);

    const result = await resp.json();
    state.lastGeneratedToken = result;
    cacheAdminToken(state, result.token_id, result.token);
    state.selectedAdminTokenID = "";
    renderAdminResult(adminResult, result);

    if (adminTenantList && !adminTenantList.hidden) await listAdminTenantTokens(false);
    else renderTokensList();

    updateAdminCopyState();
    setAdminMessage("Token generated.");
  }

  async function copyGeneratedToken() {
    let token = "";
    let label = "Token";
    let tenantID = "";

    if (state.selectedAdminTokenID) {
      token = getCachedAdminToken(state, state.selectedAdminTokenID);
      if (!token) return setAdminMessage("Selected token not available locally.", true);
      const selected = state.adminTenantTokens.find((rec) => rec.token_id === state.selectedAdminTokenID);
      tenantID = selected?.tenant_id || "";
      if (!tenantID) return setAdminMessage("Selected tenant id not available.", true);
      label = "Selected token";
    } else {
      token = state.lastGeneratedToken?.token || "";
      tenantID = state.lastGeneratedToken?.tenant_id || "";
      if (!token || !tenantID) return;
    }

    if (!navigator.clipboard?.writeText) {
      return setAdminMessage("Clipboard not available. Copy token from the result box.", true);
    }

    try {
      await navigator.clipboard.writeText(formatTenantTokenClipboard(tenantID, "token", token));
      setAdminMessage(`${label} copied (tenant + token).`);
    } catch (_err) {
      setAdminMessage("Copy failed. Copy token from the result box.", true);
    }
  }

  async function listAdminTenantTokens(showLoading = true) {
    if (!state.adminVerified) {
      setAdminMessage("Verify admin token first.", true);
      return false;
    }
    const token = adminTokenInput.value.trim();
    if (!token) {
      setAdminMessage("Admin token is required.", true);
      return false;
    }

    state.adminToken = token;
    localStorage.setItem("admin_token", state.adminToken);
    if (showLoading) setAdminMessage("Loading tenant tokens...");

    let resp;
    try {
      resp = await adminApi("/admin/tokens");
    } catch (err) {
      setAdminMessage(`Request failed: ${String(err)}`, true);
      return false;
    }
    if (!resp.ok) {
      setAdminMessage(await resp.text(), true);
      return false;
    }

    const body = await resp.json();
    const tenants = (Array.isArray(body.tokens) ? body.tokens : [])
      .filter((rec) => rec.type === "tenant")
      .sort((a, b) => Number(b.created_at_ms || 0) - Number(a.created_at_ms || 0));

    state.adminTenantTokens = tenants;
    adminTenantList.hidden = false;
    if (state.selectedAdminTokenID && !tenants.some((rec) => rec.token_id === state.selectedAdminTokenID)) {
      state.selectedAdminTokenID = "";
    }
    renderTokensList();
    updateAdminCopyState();
    setAdminMessage(`Loaded ${tenants.length} tenant token(s).`);
    return true;
  }

  async function revokeAdminToken(tokenID) {
    if (!tokenID) return;
    if (!window.confirm(`Revoke tenant token ${tokenID.slice(0, 8)}?`)) return;

    const token = adminTokenInput.value.trim();
    if (!token) return setAdminMessage("Admin token is required.", true);

    state.adminToken = token;
    localStorage.setItem("admin_token", state.adminToken);
    setAdminMessage("Revoking token...");

    const resp = await adminApi(`/admin/tokens/${encodeURIComponent(tokenID)}/revoke`, { method: "POST" });
    if (!resp.ok) return setAdminMessage(await resp.text(), true);

    for (const rec of state.adminTenantTokens) {
      if (rec.token_id === tokenID) rec.revoked = true;
    }
    renderTokensList();
    setAdminMessage("Token revoked.");
  }

  async function importAdminTokensCsv(file) {
    if (!state.adminVerified) return setAdminMessage("Verify admin token first.", true);
    if (state.adminImportInFlight) return;
    if (!file) return setAdminMessage("Select a CSV file to import.", true);

    setAdminMessage("Importing tokens...");
    setAdminImportBusy(true);

    let text = "";
    try {
      text = await file.text();
    } catch (err) {
      setAdminMessage(`Failed to read file: ${String(err)}`, true);
      setAdminImportBusy(false);
      return;
    }

    const { records, error } = buildAdminImportRecords(text);
    if (error) {
      setAdminMessage(error, true);
      renderAdminImportResult(adminImportResult, null);
      setAdminImportBusy(false);
      return;
    }
    if (!records.length) {
      setAdminMessage("No importable rows found.", true);
      renderAdminImportResult(adminImportResult, null);
      setAdminImportBusy(false);
      return;
    }

    const maxRows = 2000;
    if (records.length > maxRows) {
      setAdminMessage(`Too many rows (${records.length}). Max ${maxRows}.`, true);
      renderAdminImportResult(adminImportResult, null);
      setAdminImportBusy(false);
      return;
    }

    state.adminToken = adminTokenInput.value.trim();
    localStorage.setItem("admin_token", state.adminToken);

    let resp;
    try {
      resp = await adminApi("/admin/tokens/import", { method: "POST", body: JSON.stringify({ tokens: records }) });
    } catch (err) {
      setAdminMessage(`Request failed: ${String(err)}`, true);
      setAdminImportBusy(false);
      return;
    }
    if (!resp.ok) {
      setAdminMessage(await resp.text(), true);
      setAdminImportBusy(false);
      return;
    }

    const payload = await resp.json();
    renderAdminImportResult(adminImportResult, payload);

    const tokenByRow = new Map(records.map((rec) => [rec.row, rec.token]));
    for (const item of payload.results || []) {
      if (!item?.token_id) continue;
      const token = tokenByRow.get(item.row);
      if (token) cacheAdminToken(state, item.token_id, token);
    }

    const listOK = adminTenantList && !adminTenantList.hidden
      ? await listAdminTenantTokens(false)
      : (renderTokensList(), updateAdminCopyState(), true);

    if (listOK) {
      setAdminMessage(`Import complete: ${payload.imported || 0} imported, ${payload.skipped || 0} skipped, ${payload.errors || 0} errors.`);
    }
    setAdminImportBusy(false);
  }

  async function fetchAdminServers() {
    const resp = await adminApi("/admin/servers");
    if (!resp.ok) return;
    const body = await resp.json();
    state.adminServers = body.servers || [];
    renderAdminServers(document.getElementById("adminServersList"), state.adminServers, adminServersSearch?.value || "");
  }

  async function fetchAdminSessions() {
    const resp = await adminApi("/admin/sessions");
    if (!resp.ok) return;
    const body = await resp.json();
    state.adminSessions = body.sessions || [];
    renderAdminSessions(
      document.getElementById("adminSessionsList"),
      state.adminSessions,
      adminSessionsSearch?.value || "",
      adminSessionsStatusFilter?.value || "",
      adminStopSession,
    );
  }

  async function refreshAdminOverview() {
    const [serversResp, sessionsResp, tokensResp] = await Promise.all([
      adminApi("/admin/servers"),
      adminApi("/admin/sessions"),
      adminApi("/admin/tokens"),
    ]);

    if (serversResp.ok) state.adminServers = (await serversResp.json()).servers || [];
    if (sessionsResp.ok) state.adminSessions = (await sessionsResp.json()).sessions || [];

    let allTokens = [];
    if (tokensResp.ok) {
      allTokens = (await tokensResp.json()).tokens || [];
      state.adminTenantTokens = allTokens
        .filter((r) => r.type === "tenant")
        .sort((a, b) => Number(b.created_at_ms || 0) - Number(a.created_at_ms || 0));
    }

    renderAdminOverviewStats({ servers: state.adminServers, sessions: state.adminSessions, tokens: allTokens });
  }

  async function adminStopSession(sessionID) {
    if (!sessionID) return;
    if (!window.confirm(`Stop session ${sessionID.slice(0, 8)}?`)) return;

    const resp = await adminApi(`/admin/sessions/${encodeURIComponent(sessionID)}/stop`, {
      method: "POST",
      body: JSON.stringify({}),
    });
    if (!resp.ok) {
      alert(await resp.text());
      return;
    }
    await fetchAdminSessions();
  }

  function guardedExportJson() {
    if (!state.adminVerified) return setAdminMessage("Verify admin token first.", true);
    return exportAdminTokensJson(state, setAdminMessage);
  }

  function guardedExportCsv() {
    if (!state.adminVerified) return setAdminMessage("Verify admin token first.", true);
    return exportAdminTokensCsv(state, setAdminMessage);
  }

  return {
    createAdminToken,
    copyGeneratedToken,
    listAdminTenantTokens,
    revokeAdminToken,
    importAdminTokensCsv,
    fetchAdminServers,
    fetchAdminSessions,
    refreshAdminOverview,
    adminStopSession,
    guardedExportJson,
    guardedExportCsv,
  };
}
