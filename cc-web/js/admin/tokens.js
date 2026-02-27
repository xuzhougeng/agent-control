import { downloadBlob, timestampedFilename } from "../shared/utils.js";

const ADMIN_TOKEN_CACHE_KEY = "admin_token_cache";

export function loadAdminTokenCache() {
  try {
    const raw = localStorage.getItem(ADMIN_TOKEN_CACHE_KEY);
    if (!raw) return new Map();
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object") return new Map();
    const map = new Map();
    for (const [tokenID, token] of Object.entries(parsed)) {
      if (tokenID && typeof token === "string") map.set(tokenID, token);
    }
    return map;
  } catch (_err) {
    return new Map();
  }
}

export function persistAdminTokenCache(state) {
  try {
    const payload = {};
    for (const [tokenID, token] of state.adminTokenSecrets) payload[tokenID] = token;
    localStorage.setItem(ADMIN_TOKEN_CACHE_KEY, JSON.stringify(payload));
  } catch (_err) {
    // ignore
  }
}

export function cacheAdminToken(state, tokenID, token) {
  if (!tokenID || !token) return;
  state.adminTokenSecrets.set(tokenID, token);
  persistAdminTokenCache(state);
}

export function getCachedAdminToken(state, tokenID) {
  if (!tokenID) return "";
  return state.adminTokenSecrets.get(tokenID) || "";
}

export function toCsvValue(value) {
  const raw = value === undefined || value === null ? "" : String(value);
  const escaped = raw.replaceAll('"', '""');
  if (/[",\n]/.test(escaped)) return `"${escaped}"`;
  return escaped;
}

export function parseCsvRows(input) {
  const rows = [];
  let row = [];
  let value = "";
  let inQuotes = false;
  for (let i = 0; i < input.length; i += 1) {
    const ch = input[i];
    if (inQuotes) {
      if (ch === '"') {
        if (input[i + 1] === '"') {
          value += '"';
          i += 1;
        } else {
          inQuotes = false;
        }
      } else {
        value += ch;
      }
      continue;
    }
    if (ch === '"') {
      inQuotes = true;
      continue;
    }
    if (ch === ",") {
      row.push(value);
      value = "";
      continue;
    }
    if (ch === "\n") {
      row.push(value);
      rows.push(row);
      row = [];
      value = "";
      continue;
    }
    if (ch === "\r") continue;
    value += ch;
  }
  if (value.length || row.length) {
    row.push(value);
    rows.push(row);
  }
  return rows.filter((item) => item.some((cell) => String(cell || "").trim() !== ""));
}

export function buildAdminImportRecords(text) {
  const rows = parseCsvRows(text || "");
  if (!rows.length) return { records: [], error: "No rows found in CSV." };
  const header = rows[0].map((cell) => String(cell || "").trim().toLowerCase());
  const hasHeader = header[0] === "tenant_id" && header[1] === "token";
  const records = [];
  const startIndex = hasHeader ? 1 : 0;
  for (let i = startIndex; i < rows.length; i += 1) {
    const row = rows[i] || [];
    const tenantID = String(row[0] || "").trim();
    const token = String(row[1] || "").trim();
    if (!tenantID && !token) continue;
    records.push({ row: i + 1, tenant_id: tenantID, token });
  }
  return { records };
}

export function buildAdminExportRecords(state) {
  const records = [];
  const seen = new Set();
  for (const rec of state.adminTenantTokens) {
    if (!rec || !rec.token_id || seen.has(rec.token_id)) continue;
    seen.add(rec.token_id);
    records.push({
      tenant_id: rec.tenant_id || "",
      token_id: rec.token_id,
      token: getCachedAdminToken(state, rec.token_id) || "",
      created_at_ms: rec.created_at_ms || 0,
      revoked: Boolean(rec.revoked),
    });
  }
  if (!records.length && state.lastGeneratedToken?.token_id) {
    records.push({
      tenant_id: state.lastGeneratedToken.tenant_id || "",
      token_id: state.lastGeneratedToken.token_id,
      token: state.lastGeneratedToken.token || "",
      created_at_ms: state.lastGeneratedToken.created_at_ms || 0,
      revoked: false,
    });
  }
  return records;
}

export function exportAdminTokensCsv(state, setMessage) {
  const records = buildAdminExportRecords(state);
  if (!records.length) {
    setMessage("No tenant tokens available to export.", true);
    return;
  }
  const header = ["tenant_id", "token_id", "token", "created_at_ms", "revoked"];
  const lines = [header.map(toCsvValue).join(",")];
  for (const rec of records) {
    lines.push([
      toCsvValue(rec.tenant_id),
      toCsvValue(rec.token_id),
      toCsvValue(rec.token),
      toCsvValue(rec.created_at_ms),
      toCsvValue(rec.revoked),
    ].join(","));
  }
  downloadBlob(timestampedFilename("tenant-tokens", "csv"), lines.join("\n"), "text/csv");
  setMessage(`Exported ${records.length} token(s) to CSV.`);
}

export function exportAdminTokensJson(state, setMessage) {
  const records = buildAdminExportRecords(state);
  if (!records.length) {
    setMessage("No tenant tokens available to export.", true);
    return;
  }
  const payload = { exported_at_ms: Date.now(), tokens: records };
  downloadBlob(timestampedFilename("tenant-tokens", "json"), JSON.stringify(payload, null, 2), "application/json");
  setMessage(`Exported ${records.length} token(s).`);
}

export function formatTenantTokenClipboard(tenantID, tokenLabel, tokenValue) {
  const safeTenant = tenantID || "-";
  const safeToken = tokenValue || "";
  const label = tokenLabel || "token";
  return `tenant_id: ${safeTenant}\n${label}: ${safeToken}`;
}
