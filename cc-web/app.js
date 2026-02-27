// Legacy compatibility entrypoint.
// The UI has been modularized under /js/*. New pages load those modules directly.

(function legacyEntrypoint() {
  const hasTerminal = Boolean(document.getElementById("terminal"));
  const hasChat = Boolean(document.getElementById("chatContainer"));
  const hasAdmin = Boolean(document.getElementById("adminVerifyBtn"));
  const hasTenant = Boolean(document.getElementById("tenantVerifyBtn"));

  let modulePath = "";
  if (hasTerminal) modulePath = "/js/main-controller.js";
  else if (hasChat) modulePath = "/js/main-chat.js";
  else if (hasAdmin) modulePath = "/js/main-admin.js";
  else if (hasTenant) modulePath = "/js/main-tenant.js";

  if (!modulePath) return;

  import(modulePath).catch((err) => {
    console.error("Failed to load modular UI entrypoint", modulePath, err);
  });
})();
