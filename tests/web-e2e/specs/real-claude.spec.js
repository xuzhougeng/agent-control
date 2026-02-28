const { test, expect } = require("@playwright/test");
const fs = require("node:fs");
const path = require("node:path");

const repoRoot = process.cwd();
const uiToken = process.env.CC_WEB_E2E_UI_TOKEN || "ui-e2e-token";
const runRealClaude = process.env.CC_WEB_E2E_CLAUDE_MODE === "real";
const xtermStubJS = fs.readFileSync(path.join(repoRoot, "tests/web-e2e/fixtures/xterm-stub.js"), "utf8");
const xtermStubCSS = fs.readFileSync(path.join(repoRoot, "tests/web-e2e/fixtures/xterm-stub.css"), "utf8");

async function installXtermStub(page) {
  await page.route("https://cdn.jsdelivr.net/npm/xterm/css/xterm.css", async (route) => {
    await route.fulfill({ status: 200, contentType: "text/css", body: xtermStubCSS });
  });
  await page.route("https://cdn.jsdelivr.net/npm/xterm/lib/xterm.js", async (route) => {
    await route.fulfill({ status: 200, contentType: "application/javascript", body: xtermStubJS });
  });
  await page.route("https://cdn.jsdelivr.net/npm/xterm-addon-fit/lib/xterm-addon-fit.js", async (route) => {
    await route.fulfill({ status: 200, contentType: "application/javascript", body: "window.FitAddon = window.FitAddon || { FitAddon: class { fit() {} } };" });
  });
}

async function primeToken(page) {
  await page.addInitScript((token) => {
    localStorage.setItem("ui_token", token);
  }, uiToken);
}

async function openLeftDrawer(page) {
  await page.evaluate(() => {
    const body = document.body;
    const leftBtn = document.getElementById("leftDrawerToggleBtn");
    const topBtn = document.getElementById("sidebarToggleBtn");
    const backdrop = document.getElementById("sidebarBackdrop");
    body.classList.add("left-drawer-open");
    leftBtn?.setAttribute("aria-expanded", "true");
    topBtn?.setAttribute("aria-expanded", "true");
    if (backdrop) backdrop.hidden = false;
  });
}

async function closeLeftDrawer(page) {
  await page.evaluate(() => {
    const body = document.body;
    const leftBtn = document.getElementById("leftDrawerToggleBtn");
    const topBtn = document.getElementById("sidebarToggleBtn");
    const backdrop = document.getElementById("sidebarBackdrop");
    body.classList.remove("left-drawer-open");
    leftBtn?.setAttribute("aria-expanded", "false");
    topBtn?.setAttribute("aria-expanded", "false");
    if (backdrop) backdrop.hidden = !body.classList.contains("right-drawer-open");
  });
}

async function openRightDrawer(page) {
  await page.evaluate(() => {
    const body = document.body;
    const rightBtn = document.getElementById("rightDrawerToggleBtn");
    const backdrop = document.getElementById("sidebarBackdrop");
    body.classList.add("right-drawer-open");
    rightBtn?.setAttribute("aria-expanded", "true");
    if (backdrop) backdrop.hidden = false;
  });
}

async function closeRightDrawer(page) {
  await page.evaluate(() => {
    const body = document.body;
    const rightBtn = document.getElementById("rightDrawerToggleBtn");
    const backdrop = document.getElementById("sidebarBackdrop");
    body.classList.remove("right-drawer-open");
    rightBtn?.setAttribute("aria-expanded", "false");
    if (backdrop) backdrop.hidden = !body.classList.contains("left-drawer-open");
  });
}

async function waitForWorkspaceReady(page) {
  await expect(page.locator("#wsStatus")).toContainText("connected");
  await openLeftDrawer(page);
  await expect(page.locator("#serversList")).toContainText("srv-e2e");
  await closeLeftDrawer(page);
}

async function createSession(page, { cwd } = {}) {
  logStep("create-session:start");
  const root = cwd || repoRoot;
  await openLeftDrawer(page);
  logStep("create-session:left-drawer-open");
  const toolsSummary = page.locator("#workspaceToolsDetails summary");
  const toolsDetails = page.locator("#workspaceToolsDetails");
  if (!(await toolsDetails.evaluate((el) => el.hasAttribute("open")))) {
    await toolsSummary.click({ timeout: 10_000 });
    logStep("create-session:tools-opened");
  }
  await page.getByLabel("cwd").fill(root, { timeout: 10_000 });
  logStep("create-session:cwd-filled");
  await page.getByRole("button", { name: "Create" }).click({ timeout: 10_000 });
  logStep("create-session:create-clicked");
  await expect(page.locator("#workspaceSessionTitle")).not.toHaveText("No session selected", { timeout: 30_000 });
  logStep("create-session:title-ready");
  const sessionID = String((await page.locator("#workspaceSessionTitle").textContent()) || "").trim();
  const shortSessionID = sessionID.slice(0, 8);
  const sessionRow = shortSessionID
    ? page.locator("#sessionsList li.session-item").filter({ hasText: shortSessionID }).first()
    : page.locator("#sessionsList li.session-item").first();
  await expect(sessionRow).toBeVisible({ timeout: 20_000 });
  await page.evaluate((shortID) => {
    const rows = Array.from(document.querySelectorAll("#sessionsList li.session-item"));
    const target = rows.find((row) => !shortID || String(row.textContent || "").includes(shortID)) || rows[0];
    if (!target) return;
    target.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true, button: 0 }));
  }, shortSessionID);
  logStep("create-session:session-row-clicked");
  await closeLeftDrawer(page);
  logStep(`create-session:done:${sessionID}`);
  return sessionID;
}

async function closeSessionCreationPanel(page) {
  await page.evaluate(() => {
    const details = document.getElementById("workspaceToolsDetails");
    if (details && details.hasAttribute("open")) {
      details.removeAttribute("open");
    }
    const body = document.body;
    body.classList.remove("left-drawer-open");
    const leftBtn = document.getElementById("leftDrawerToggleBtn");
    const topBtn = document.getElementById("sidebarToggleBtn");
    leftBtn?.setAttribute("aria-expanded", "false");
    topBtn?.setAttribute("aria-expanded", "false");
    const backdrop = document.getElementById("sidebarBackdrop");
    if (backdrop) backdrop.hidden = !body.classList.contains("right-drawer-open");
  });
  logStep("create-session:panel-closed");
}

function sanitizeStepLabel(label) {
  return String(label).toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
}

function logStep(label) {
  const stamp = new Date().toISOString();
  // Keep logs plain for CI/stdout readability.
  console.log(`[real-claude][${stamp}] ${label}`);
}

async function captureStep(page, testInfo, label, options = {}) {
  logStep(`capture:start:${label}`);
  const withDrawers = options.withDrawers !== false;
  let snapshotState = null;
  if (withDrawers) {
    snapshotState = await page.evaluate(() => {
      const body = document.body;
      const leftBtn = document.getElementById("leftDrawerToggleBtn");
      const rightBtn = document.getElementById("rightDrawerToggleBtn");
      const topBtn = document.getElementById("sidebarToggleBtn");
      return {
        leftOpen: body.classList.contains("left-drawer-open"),
        rightOpen: body.classList.contains("right-drawer-open"),
        mobileOpen: body.classList.contains("sidebar-open"),
        leftExpanded: leftBtn?.getAttribute("aria-expanded") || "false",
        rightExpanded: rightBtn?.getAttribute("aria-expanded") || "false",
        topExpanded: topBtn?.getAttribute("aria-expanded") || "false",
      };
    });

    await page.evaluate(() => {
      const body = document.body;
      const leftBtn = document.getElementById("leftDrawerToggleBtn");
      const rightBtn = document.getElementById("rightDrawerToggleBtn");
      const topBtn = document.getElementById("sidebarToggleBtn");
      const backdrop = document.getElementById("sidebarBackdrop");
      body.classList.add("left-drawer-open", "right-drawer-open");
      leftBtn?.setAttribute("aria-expanded", "true");
      rightBtn?.setAttribute("aria-expanded", "true");
      topBtn?.setAttribute("aria-expanded", "true");
      if (backdrop) backdrop.hidden = false;
    });
  }
  await page.screenshot({
    path: testInfo.outputPath(`${sanitizeStepLabel(label)}.png`),
    fullPage: false,
    animations: "disabled",
    timeout: 15_000,
  });
  if (withDrawers && snapshotState) {
    await page.evaluate((state) => {
      const body = document.body;
      const leftBtn = document.getElementById("leftDrawerToggleBtn");
      const rightBtn = document.getElementById("rightDrawerToggleBtn");
      const topBtn = document.getElementById("sidebarToggleBtn");
      const backdrop = document.getElementById("sidebarBackdrop");

      body.classList.toggle("left-drawer-open", Boolean(state.leftOpen));
      body.classList.toggle("right-drawer-open", Boolean(state.rightOpen));
      body.classList.toggle("sidebar-open", Boolean(state.mobileOpen));
      leftBtn?.setAttribute("aria-expanded", String(state.leftExpanded));
      rightBtn?.setAttribute("aria-expanded", String(state.rightExpanded));
      topBtn?.setAttribute("aria-expanded", String(state.topExpanded));
      if (backdrop) backdrop.hidden = !(state.leftOpen || state.rightOpen || state.mobileOpen);
    }, snapshotState);
  }
  logStep(`capture:end:${label}`);
}

test.beforeEach(async ({ page }) => {
  test.skip(!runRealClaude, "set CC_WEB_E2E_CLAUDE_MODE=real to run the real Claude smoke test");
  await page.setViewportSize({ width: 1720, height: 1080 });
  await installXtermStub(page);
  await primeToken(page);
});

test("real Claude flow: terminal hi then switch chat and send hi without exited", async ({ page }, testInfo) => {
  test.slow();
  test.setTimeout(300_000);
  logStep("test:start");

  await page.goto("/");
  logStep("workspace:loaded");
  await waitForWorkspaceReady(page);
  logStep("workspace:ready");
  await captureStep(page, testInfo, "01-workspace-ready");

  logStep("create-session:invoke");
  const sessionID = await createSession(page);
  logStep(`session:created:${sessionID}`);
  await expect(page.locator("#workspaceModeBadge")).toContainText("Terminal");
  await captureStep(page, testInfo, "02-session-created-terminal", { withDrawers: false });
  await closeSessionCreationPanel(page);

  await page.locator("#workspaceOpenTerminalBtn").click();
  await expect(page.locator("#workspaceViewBadge")).toContainText("Terminal View");
  await page.waitForTimeout(5_000);
  await captureStep(page, testInfo, "03-terminal-wait-5s-before-hi", { withDrawers: false });

  await page.evaluate(() => window.__CC_E2E__.sendTerminalInput("hi\r"));
  await expect(page.locator("#currentSessionLabel")).toContainText("Session:");
  await captureStep(page, testInfo, "04-sent-hi-from-terminal", { withDrawers: false });

  await page.waitForTimeout(10_000);
  await captureStep(page, testInfo, "05-after-10s-wait", { withDrawers: false });

  await page.locator("#workspaceOpenChatBtn").click();
  await expect(page).toHaveURL(/\/\?.*view=chat/);
  await expect(page.locator("#workspaceModeBadge")).toContainText("Terminal");
  await captureStep(page, testInfo, "06-chat-view-before-switch", { withDrawers: false });

  await page.locator("#workspaceSwitchModeBtn").click();
  await expect(page.locator("#workspaceModeBadge")).toContainText("Chat", { timeout: 120_000 });
  await expect(page.locator("#workspaceSessionTitle")).toHaveText(sessionID);
  await captureStep(page, testInfo, "07-chat-active", { withDrawers: false });

  await page.locator("#chatInput").fill("hi");
  await page.locator("#chatSendBtn").click();
  logStep("chat:sent:hi");
  await captureStep(page, testInfo, "08-chat-sent-hi", { withDrawers: false });

  await expect(page.locator("#chatMessages")).toContainText("hi", { timeout: 120_000 });
  await page.waitForTimeout(12_000);

  await expect(page.locator("#workspaceStatusBadge")).not.toContainText("exited");
  await expect(page.locator("#workspaceStatusBadge")).not.toContainText("error");
  await expect(page.locator("#chatRunState")).not.toContainText("Execution failed");
  await captureStep(page, testInfo, "09-chat-still-running", { withDrawers: false });
  logStep("test:done");
});
