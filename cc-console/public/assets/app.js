let currentEmail = "";

function showMsg(id, msg, type = "error") {
  const el = document.getElementById(id);
  el.innerHTML = `<div class="msg msg-${type}">${msg}</div>`;
  if (type === "success") setTimeout(() => (el.innerHTML = ""), 4000);
}

function clearMsg(id) { document.getElementById(id).innerHTML = ""; }

function showView(id) {
  document.querySelectorAll(".view").forEach((v) => v.classList.remove("active"));
  document.getElementById(id).classList.add("active");
}

async function sendCode() {
  const email = document.getElementById("emailInput").value.trim();
  if (!email) return showMsg("loginMsg", "Please enter your email.");
  clearMsg("loginMsg");

  const btn = document.getElementById("sendCodeBtn");
  btn.disabled = true;
  btn.innerHTML = '<span class="spinner"></span> Sending…';

  try {
    const res = await fetch("/api/auth/send-code", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email }),
    });
    const data = await res.json();
    if (!res.ok) return showMsg("loginMsg", data.error || "Failed to send code");
    currentEmail = email;
    document.getElementById("emailStep").style.display = "none";
    document.getElementById("codeStep").style.display = "block";
    document.getElementById("codeInput").focus();
    showMsg("loginMsg", `Code sent to ${email}`, "success");
  } catch (e) {
    showMsg("loginMsg", "Network error. Please try again.");
  } finally {
    btn.disabled = false;
    btn.textContent = "Send Code";
  }
}

async function verifyCode() {
  const code = document.getElementById("codeInput").value.trim();
  if (!code || code.length !== 6) return showMsg("loginMsg", "Enter the 6-digit code.");
  clearMsg("loginMsg");

  const btn = document.getElementById("verifyBtn");
  btn.disabled = true;
  btn.innerHTML = '<span class="spinner"></span> Verifying…';

  try {
    const res = await fetch("/api/auth/verify", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email: currentEmail, code }),
    });
    const data = await res.json();
    if (!res.ok) {
      const msg = res.status === 503
        ? 'Tenant pool exhausted. Please <a href="https://github.com/xuzhougeng/agent-control" target="_blank" style="color:var(--accent)">deploy your own instance</a>.'
        : (data.error || "Verification failed");
      return showMsg("loginMsg", msg);
    }
    showDashboard(data.user, data.web_url);
  } catch (e) {
    showMsg("loginMsg", "Network error.");
  } finally {
    btn.disabled = false;
    btn.textContent = "Verify";
  }
}

function backToEmail() {
  document.getElementById("emailStep").style.display = "block";
  document.getElementById("codeStep").style.display = "none";
  clearMsg("loginMsg");
}

function showDashboard(user, webUrl) {
  document.getElementById("headerEmail").textContent = user.email;
  document.getElementById("logoutBtn").style.display = "inline-flex";
  document.getElementById("acctEmail").textContent = user.email;
  document.getElementById("acctTenant").textContent = user.tenant_id || "Provisioning…";
  if (user.tenant_token) {
    document.getElementById("tenantTokenValue").textContent = user.tenant_token;
    document.getElementById("tenantTokenSection").style.display = "block";
  } else {
    document.getElementById("tenantTokenSection").style.display = "none";
    showMsg("tenantTokenMsg", "No tenant provisioned. Contact support.");
  }
  if (webUrl) {
    document.getElementById("tenantPanelLink").href = webUrl + "/tenant";
  }
  showView("dashboardView");
}

function copyToken(id) {
  const text = document.getElementById(id).textContent;
  navigator.clipboard.writeText(text).then(() => {
    const btn = document.getElementById(id).parentElement.querySelector(".copy-btn");
    btn.textContent = "Copied!";
    setTimeout(() => (btn.textContent = "Copy"), 2000);
  });
}

async function logout() {
  await fetch("/api/auth/logout", { method: "POST" });
  document.getElementById("headerEmail").textContent = "";
  document.getElementById("logoutBtn").style.display = "none";
  document.getElementById("tenantTokenSection").style.display = "none";
  document.getElementById("codeInput").value = "";
  backToEmail();
  showView("loginView");
}

async function checkSession() {
  try {
    const res = await fetch("/api/auth/me");
    if (res.ok) {
      const data = await res.json();
      showDashboard(data.user, data.web_url);
    }
  } catch (_) {}
}

document.getElementById("emailInput").addEventListener("keydown", (e) => {
  if (e.key === "Enter") sendCode();
});
document.getElementById("codeInput").addEventListener("keydown", (e) => {
  if (e.key === "Enter") verifyCode();
});

checkSession();
