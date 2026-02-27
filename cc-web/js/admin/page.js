import { createAdminApi } from "../shared/http.js";
import { debounce } from "../shared/utils.js";
import { createMessageBinder } from "../shared/messages.js";
import {
  renderAdminImportResult,
  renderAdminResult,
  renderAdminServers,
  renderAdminSessions,
  renderAdminTenantTokens,
} from "./render.js";
import { getCachedAdminToken, loadAdminTokenCache } from "./tokens.js";
import { createAdminActions } from "./actions.js";

export function initAdminPage() {
  const state = {
    adminToken: localStorage.getItem("admin_token") || "admin-dev-token",
    adminVerified: false,
    adminVerifyInFlight: false,
    adminImportInFlight: false,
    adminTenantTokens: [],
    selectedAdminTokenID: "",
    lastGeneratedToken: null,
    adminTokenSecrets: loadAdminTokenCache(),
    adminServers: [],
    adminSessions: [],
  };

  const adminApi = createAdminApi(() => state.adminToken);

  const adminTokenInput = document.getElementById("adminTokenInput");
  const adminGenerateBtn = document.getElementById("adminGenerateBtn");
  const adminCopyTokenBtn = document.getElementById("adminCopyTokenBtn");
  const adminListTenantsBtn = document.getElementById("adminListTenantsBtn");
  const adminExportBtn = document.getElementById("adminExportBtn");
  const adminExportCsvBtn = document.getElementById("adminExportCsvBtn");
  const adminImportBtn = document.getElementById("adminImportBtn");
  const adminImportInput = document.getElementById("adminImportInput");
  const adminImportResult = document.getElementById("adminImportResult");
  const adminMessage = document.getElementById("adminMessage");
  const adminResult = document.getElementById("adminResult");
  const adminTenantList = document.getElementById("adminTenantList");
  const adminVerifyBtn = document.getElementById("adminVerifyBtn");
  const adminGateMessage = document.getElementById("adminGateMessage");
  const adminContent = document.getElementById("adminContent");
  const adminServersSearch = document.getElementById("adminServersSearch");
  const adminSessionsSearch = document.getElementById("adminSessionsSearch");
  const adminSessionsStatusFilter = document.getElementById("adminSessionsStatusFilter");
  const adminTokenSearch = document.getElementById("adminTokenSearch");

  const setAdminMessage = createMessageBinder(adminMessage);
  const setGateMessage = createMessageBinder(adminGateMessage);

  let actions = null;

  function updateAdminCopyState() {
    const selected = getCachedAdminToken(state, state.selectedAdminTokenID);
    const latest = state.lastGeneratedToken?.token;
    adminCopyTokenBtn.disabled = state.selectedAdminTokenID ? !selected : !latest;
  }

  function renderTokensList() {
    renderAdminTenantTokens(adminTenantList, state, {
      adminTokenSearch,
      updateAdminCopyState,
      onToggleSelect: (tokenID) => {
        state.selectedAdminTokenID = state.selectedAdminTokenID === tokenID ? "" : tokenID;
        renderTokensList();
      },
      onRevoke: (tokenID) => actions.revokeAdminToken(tokenID),
    });
  }

  function setAdminImportBusy(busy) {
    state.adminImportInFlight = busy;
    adminImportBtn.disabled = busy || !state.adminVerified;
    adminImportInput.disabled = busy || !state.adminVerified;
  }

  function renderAdminServersFromState() {
    renderAdminServers(document.getElementById("adminServersList"), state.adminServers, adminServersSearch?.value || "");
  }

  function renderAdminSessionsFromState() {
    renderAdminSessions(
      document.getElementById("adminSessionsList"),
      state.adminSessions,
      adminSessionsSearch?.value || "",
      adminSessionsStatusFilter?.value || "",
      (sessionID) => actions.adminStopSession(sessionID),
    );
  }

  function setAdminVerified(verified, message = "", isError = false) {
    state.adminVerified = verified;
    adminContent.hidden = !verified;
    adminVerifyBtn.textContent = verified ? "Re-Verify Token" : "Verify Token";
    setGateMessage(message, isError);

    adminListTenantsBtn.disabled = !verified;
    adminExportBtn.disabled = !verified;
    adminExportCsvBtn.disabled = !verified;
    adminImportBtn.disabled = !verified;
    adminImportInput.disabled = !verified;

    if (!verified) {
      setAdminMessage("");
      adminCopyTokenBtn.disabled = true;
      state.lastGeneratedToken = null;
      state.adminTenantTokens = [];
      state.adminServers = [];
      state.adminSessions = [];
      state.selectedAdminTokenID = "";
      state.adminImportInFlight = false;
      adminTenantList.hidden = true;
      renderAdminResult(adminResult, null);
      renderTokensList();
      renderAdminImportResult(adminImportResult, null);
      return;
    }

    actions.refreshAdminOverview();
  }

  async function verifyAdminToken() {
    const token = adminTokenInput.value.trim();
    if (!token) return setAdminVerified(false, "Admin token is required.", true);
    if (state.adminVerifyInFlight) return;

    state.adminVerifyInFlight = true;
    adminVerifyBtn.disabled = true;
    adminContent.hidden = true;
    setGateMessage("Verifying...");

    state.adminToken = token;
    localStorage.setItem("admin_token", state.adminToken);

    let resp;
    try {
      resp = await adminApi("/admin/verify");
    } catch (err) {
      setAdminVerified(false, `Request failed: ${String(err)}`, true);
      adminVerifyBtn.disabled = false;
      state.adminVerifyInFlight = false;
      return;
    }

    if (!resp.ok) setAdminVerified(false, (await resp.text()) || "Unauthorized.", true);
    else setAdminVerified(true, "Verified.");

    adminVerifyBtn.disabled = false;
    state.adminVerifyInFlight = false;
  }

  function switchAdminTab(tabName) {
    for (const tab of document.querySelectorAll(".admin-tab")) {
      tab.classList.toggle("active", tab.dataset.tab === tabName);
    }
    for (const panel of document.querySelectorAll(".admin-panel")) {
      panel.classList.toggle("active", panel.id === `adminPanel${tabName.charAt(0).toUpperCase() + tabName.slice(1)}`);
    }

    if (tabName === "overview") actions.refreshAdminOverview();
    else if (tabName === "servers") actions.fetchAdminServers();
    else if (tabName === "sessions") actions.fetchAdminSessions();
    else if (tabName === "tokens" && !adminTenantList.hidden) actions.listAdminTenantTokens(false);
  }

  actions = createAdminActions({
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
  });

  adminTokenInput.value = state.adminToken;
  renderAdminResult(adminResult, null);
  renderAdminImportResult(adminImportResult, null);

  adminVerifyBtn.addEventListener("click", verifyAdminToken);
  adminTokenInput.addEventListener("input", () => {
    if (state.adminVerified) setAdminVerified(false, "Token changed. Please verify again.");
    else setGateMessage("");
  });

  adminGenerateBtn.addEventListener("click", actions.createAdminToken);
  adminCopyTokenBtn.addEventListener("click", actions.copyGeneratedToken);
  adminListTenantsBtn.addEventListener("click", () => actions.listAdminTenantTokens());
  adminExportBtn.addEventListener("click", actions.guardedExportJson);
  adminExportCsvBtn.addEventListener("click", actions.guardedExportCsv);

  adminImportBtn.addEventListener("click", () => {
    if (!state.adminVerified) return setAdminMessage("Verify admin token first.", true);
    adminImportInput.click();
  });
  adminImportInput.addEventListener("change", () => {
    const file = adminImportInput.files && adminImportInput.files[0];
    adminImportInput.value = "";
    if (file) actions.importAdminTokensCsv(file);
  });

  for (const tab of document.querySelectorAll(".admin-tab")) {
    tab.addEventListener("click", () => switchAdminTab(tab.dataset.tab));
  }

  document.getElementById("adminRefreshOverviewBtn")?.addEventListener("click", actions.refreshAdminOverview);
  document.getElementById("adminRefreshServersBtn")?.addEventListener("click", actions.fetchAdminServers);
  document.getElementById("adminRefreshSessionsBtn")?.addEventListener("click", actions.fetchAdminSessions);

  adminServersSearch?.addEventListener("input", debounce(renderAdminServersFromState, 200));
  adminSessionsSearch?.addEventListener("input", debounce(renderAdminSessionsFromState, 200));
  adminSessionsStatusFilter?.addEventListener("change", renderAdminSessionsFromState);
  adminTokenSearch?.addEventListener("input", debounce(renderTokensList, 200));

  setAdminVerified(false, "Enter admin token to continue.");
  if (adminTokenInput.value.trim()) verifyAdminToken();
}
