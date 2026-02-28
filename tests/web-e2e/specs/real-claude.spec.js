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
  const btn = page.locator("#leftDrawerToggleBtn");
  if ((await btn.getAttribute("aria-expanded")) !== "true") {
    await btn.click();
  }
}

async function closeLeftDrawer(page) {
  const btn = page.locator("#leftDrawerToggleBtn");
  if ((await btn.getAttribute("aria-expanded")) === "true") {
    await btn.click();
  }
}

async function openRightDrawer(page) {
  const btn = page.locator("#rightDrawerToggleBtn");
  if ((await btn.getAttribute("aria-expanded")) !== "true") {
    await btn.click();
  }
}

async function closeRightDrawer(page) {
  const btn = page.locator("#rightDrawerToggleBtn");
  if ((await btn.getAttribute("aria-expanded")) === "true") {
    await btn.click();
  }
}

async function waitForWorkspaceReady(page) {
  await expect(page.locator("#wsStatus")).toContainText("connected");
  await openLeftDrawer(page);
  await expect(page.locator("#serversList")).toContainText("srv-e2e");
  await closeLeftDrawer(page);
}

async function createSession(page, { cwd } = {}) {
  const root = cwd || repoRoot;
  await openLeftDrawer(page);
  const toolsSummary = page.locator("#workspaceToolsDetails summary");
  const toolsDetails = page.locator("#workspaceToolsDetails");
  if (!(await toolsDetails.evaluate((el) => el.hasAttribute("open")))) {
    await toolsSummary.click();
  }
  await page.getByLabel("cwd").fill(root);
  await page.getByRole("button", { name: "Create" }).click();
  await expect(page.locator("#workspaceSessionTitle")).not.toHaveText("No session selected");
  const sessionID = String((await page.locator("#workspaceSessionTitle").textContent()) || "").trim();
  const shortSessionID = sessionID.slice(0, 8);
  const sessionRow = shortSessionID
    ? page.locator("#sessionsList li.session-item").filter({ hasText: shortSessionID }).first()
    : page.locator("#sessionsList li.session-item").first();
  await expect(sessionRow).toBeVisible();
  await sessionRow.click();
  await closeLeftDrawer(page);
  return sessionID;
}

function sanitizeStepLabel(label) {
  return String(label).toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
}

async function captureStep(page, testInfo, label, options = {}) {
  const withDrawers = options.withDrawers !== false;
  if (withDrawers) {
    await openLeftDrawer(page);
    await openRightDrawer(page);
  }
  await page.screenshot({
    path: testInfo.outputPath(`${sanitizeStepLabel(label)}.png`),
    fullPage: true,
  });
  if (withDrawers) {
    await closeRightDrawer(page);
    await closeLeftDrawer(page);
  }
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

  await page.goto("/");
  await waitForWorkspaceReady(page);
  await captureStep(page, testInfo, "01-workspace-ready");

  const sessionID = await createSession(page);
  await expect(page.locator("#workspaceModeBadge")).toContainText("Terminal");
  await captureStep(page, testInfo, "02-session-created-terminal");

  await page.locator("#workspaceOpenTerminalBtn").click();
  await expect(page.locator("#workspaceViewBadge")).toContainText("Terminal View");
  await page.waitForTimeout(5_000);
  await captureStep(page, testInfo, "03-terminal-wait-5s-before-hi");

  await page.evaluate(() => window.__CC_E2E__.sendTerminalInput("hi\r"));
  await expect(page.locator("#currentSessionLabel")).toContainText("Session:");
  await captureStep(page, testInfo, "04-sent-hi-from-terminal");

  await page.waitForTimeout(10_000);
  await captureStep(page, testInfo, "05-after-10s-wait");

  await page.locator("#workspaceOpenChatBtn").click();
  await expect(page).toHaveURL(/\/\?.*view=chat/);
  await expect(page.locator("#workspaceModeBadge")).toContainText("Terminal");
  await captureStep(page, testInfo, "06-chat-view-before-switch");

  await page.locator("#workspaceSwitchModeBtn").click();
  await expect(page.locator("#workspaceModeBadge")).toContainText("Chat", { timeout: 120_000 });
  await expect(page.locator("#workspaceSessionTitle")).toHaveText(sessionID);
  await captureStep(page, testInfo, "07-chat-active");

  await page.locator("#chatInput").fill("hi");
  await page.locator("#chatSendBtn").click();
  await captureStep(page, testInfo, "08-chat-sent-hi");

  await expect(page.locator("#chatMessages")).toContainText("hi", { timeout: 120_000 });
  await page.waitForTimeout(12_000);

  await expect(page.locator("#workspaceStatusBadge")).not.toContainText("exited");
  await expect(page.locator("#workspaceStatusBadge")).not.toContainText("error");
  await expect(page.locator("#chatRunState")).not.toContainText("Execution failed");
  await captureStep(page, testInfo, "09-chat-still-running");
});
