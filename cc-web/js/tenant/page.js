import { createTenantApi } from "../shared/http.js";
import { formatTime, normalizeValue } from "../shared/utils.js";
import { createMessageBinder } from "../shared/messages.js";

export function initTenantPage() {
  const state = {
    tenantToken: localStorage.getItem("tenant_token") || "",
    tenantID: localStorage.getItem("tenant_id") || "",
    tenantVerified: false,
    tenantVerifyInFlight: false,
    lastTenantTokens: null,
  };

  const tenantApi = createTenantApi(() => state.tenantToken);

  const tenantTokenInput = document.getElementById("tenantTokenInput");
  const tenantTenantInput = document.getElementById("tenantTenantInput");
  const tenantRoleSelect = document.getElementById("tenantRoleSelect");
  const tenantGenerateBtn = document.getElementById("tenantGenerateBtn");
  const tenantCopyUiTokenBtn = document.getElementById("tenantCopyUiTokenBtn");
  const tenantCopyAgentTokenBtn = document.getElementById("tenantCopyAgentTokenBtn");
  const tenantMessage = document.getElementById("tenantMessage");
  const tenantResult = document.getElementById("tenantResult");
  const tenantVerifyBtn = document.getElementById("tenantVerifyBtn");
  const tenantGateMessage = document.getElementById("tenantGateMessage");
  const tenantContent = document.getElementById("tenantContent");

  const setTenantMessage = createMessageBinder(tenantMessage);
  const setGateMessage = createMessageBinder(tenantGateMessage);

  function makeRow(label, value, mono = false) {
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

  function renderTenantResult(data) {
    tenantResult.innerHTML = "";
    if (!data) {
      const empty = document.createElement("div");
      empty.className = "admin-result-empty";
      empty.textContent = "(no token generated)";
      tenantResult.appendChild(empty);
      return;
    }

    const card = document.createElement("div");
    card.className = "admin-result-card";
    const header = document.createElement("div");
    header.className = "admin-result-header";
    const title = document.createElement("strong");
    title.textContent = "Latest Tenant Tokens";
    header.appendChild(title);

    const fields = document.createElement("div");
    fields.className = "admin-field-list";
    fields.appendChild(makeRow("tenant", data.tenant_id, true));
    fields.appendChild(makeRow("revoked", data.revoked_count));
    fields.appendChild(makeRow("ui_token", data.ui?.token, true));
    fields.appendChild(makeRow("ui_token_id", data.ui?.token_id, true));
    fields.appendChild(makeRow("ui_role", data.ui?.role));
    fields.appendChild(makeRow("ui_created", formatTime(data.ui?.created_at_ms)));
    fields.appendChild(makeRow("agent_token", data.agent?.token, true));
    fields.appendChild(makeRow("agent_token_id", data.agent?.token_id, true));
    fields.appendChild(makeRow("agent_created", formatTime(data.agent?.created_at_ms)));

    card.appendChild(header);
    card.appendChild(fields);
    tenantResult.appendChild(card);
  }

  function setTenantVerified(verified, message = "", isError = false) {
    state.tenantVerified = verified;
    tenantContent.hidden = !verified;
    tenantVerifyBtn.textContent = verified ? "Re-Verify Token" : "Verify Token";
    setGateMessage(message, isError);
    if (!verified) {
      setTenantMessage("");
      tenantCopyUiTokenBtn.disabled = true;
      tenantCopyAgentTokenBtn.disabled = true;
      state.lastTenantTokens = null;
      renderTenantResult(null);
    }
  }

  async function verifyTenantToken() {
    const token = tenantTokenInput.value.trim();
    const tenantID = tenantTenantInput.value.trim();
    if (!token) return setTenantVerified(false, "Tenant token is required.", true);
    if (!tenantID) return setTenantVerified(false, "Tenant id is required.", true);
    if (state.tenantVerifyInFlight) return;

    state.tenantVerifyInFlight = true;
    tenantVerifyBtn.disabled = true;
    tenantContent.hidden = true;
    setGateMessage("Verifying...");

    state.tenantToken = token;
    localStorage.setItem("tenant_token", state.tenantToken);
    state.tenantID = tenantID;
    localStorage.setItem("tenant_id", state.tenantID);

    let resp;
    try {
      resp = await tenantApi("/tenant/verify", {
        method: "POST",
        body: JSON.stringify({ tenant_id: tenantID }),
      });
    } catch (err) {
      setTenantVerified(false, `Request failed: ${String(err)}`, true);
      tenantVerifyBtn.disabled = false;
      state.tenantVerifyInFlight = false;
      return;
    }

    if (!resp.ok) {
      const msg = (await resp.text()) || "Unauthorized.";
      setTenantVerified(false, msg, true);
    } else {
      const body = await resp.json().catch(() => ({}));
      if (body?.tenant_id && tenantTenantInput.value.trim() !== body.tenant_id) {
        tenantTenantInput.value = body.tenant_id;
      }
      setTenantVerified(true, "Verified.");
    }

    tenantVerifyBtn.disabled = false;
    state.tenantVerifyInFlight = false;
  }

  async function createTenantTokens() {
    const token = tenantTokenInput.value.trim();
    const tenantID = tenantTenantInput.value.trim();
    if (!token) return setTenantMessage("Tenant token is required.", true);
    if (!state.tenantVerified) return setTenantMessage("Verify tenant token first.", true);
    if (!tenantID) return setTenantMessage("Tenant id is required.", true);

    state.tenantToken = token;
    localStorage.setItem("tenant_token", state.tenantToken);

    setTenantMessage("Generating tokens...");
    let resp;
    try {
      resp = await tenantApi("/tenant/tokens", {
        method: "POST",
        body: JSON.stringify({ tenant_id: tenantID, role: tenantRoleSelect.value }),
      });
    } catch (err) {
      setTenantMessage(`Request failed: ${String(err)}`, true);
      return;
    }
    if (!resp.ok) {
      setTenantMessage(await resp.text(), true);
      return;
    }

    const result = await resp.json();
    state.lastTenantTokens = result;
    if (result.tenant_id) tenantTenantInput.value = result.tenant_id;
    tenantCopyUiTokenBtn.disabled = !(result.ui && result.ui.token);
    tenantCopyAgentTokenBtn.disabled = !(result.agent && result.agent.token);
    renderTenantResult(result);
    setTenantMessage("Tokens generated.");
  }

  async function copyTenantToken(kind) {
    const result = state.lastTenantTokens || {};
    const token = kind === "agent" ? result.agent?.token : result.ui?.token;
    if (!token) return;
    if (!navigator.clipboard?.writeText) {
      setTenantMessage("Clipboard not available. Copy token from the result box.", true);
      return;
    }
    try {
      await navigator.clipboard.writeText(token);
      setTenantMessage(`${kind === "agent" ? "Agent" : "UI"} token copied to clipboard.`);
    } catch (_err) {
      setTenantMessage("Copy failed. Copy token from the result box.", true);
    }
  }

  tenantTokenInput.value = state.tenantToken;
  tenantTenantInput.value = state.tenantID;
  tenantVerifyBtn.addEventListener("click", verifyTenantToken);
  tenantTokenInput.addEventListener("input", () => {
    if (state.tenantVerified) setTenantVerified(false, "Token changed. Please verify again.");
    else setGateMessage("");
  });
  tenantTenantInput.addEventListener("input", () => {
    if (state.tenantVerified) setTenantVerified(false, "Tenant changed. Please verify again.");
    else setGateMessage("");
  });
  tenantGenerateBtn.addEventListener("click", createTenantTokens);
  tenantCopyUiTokenBtn.addEventListener("click", () => copyTenantToken("ui"));
  tenantCopyAgentTokenBtn.addEventListener("click", () => copyTenantToken("agent"));

  renderTenantResult(null);
  setTenantVerified(false, "Enter tenant id + token to continue.");
  if (tenantTokenInput.value.trim() && tenantTenantInput.value.trim()) {
    verifyTenantToken();
  }
}
