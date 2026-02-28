const { test, expect } = require("@playwright/test");
const fs = require("node:fs");
const path = require("node:path");

const repoRoot = process.cwd();
const uiToken = process.env.CC_WEB_E2E_UI_TOKEN || "ui-e2e-token";
const preseededSessionID = "11111111-1111-4111-8111-111111111111";
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

async function waitForWorkspaceReady(page) {
  await expect(page.locator("#wsStatus")).toContainText("connected");
  await expect(page.locator("#serversList")).toContainText("srv-e2e");
}

async function createSession(page, { cwd, sessionID } = {}) {
  const root = cwd || repoRoot;
  const toolsSummary = page.locator("#workspaceToolsDetails summary");
  const toolsDetails = page.locator("#workspaceToolsDetails");
  if (!(await toolsDetails.evaluate((el) => el.hasAttribute("open")))) {
    await toolsSummary.click();
  }
  await page.getByLabel("cwd").fill(root);
  if (sessionID) {
    await page.getByLabel(/session id/i).fill(sessionID);
  }
  await page.getByRole("button", { name: "Create" }).click();
  await expect(page.locator("#workspaceSessionTitle")).not.toHaveText("No session selected");
  return page.locator("#workspaceSessionTitle").textContent();
}

test.beforeEach(async ({ page }) => {
  await installXtermStub(page);
  await primeToken(page);
});

test("chat workspace creates a unified session and sends a message", async ({ page }) => {
  await page.goto("/?view=chat");
  await waitForWorkspaceReady(page);

  const sessionID = await createSession(page);
  await expect(page.locator("#workspaceModeBadge")).toContainText("Chat");

  await page.locator("#chatInput").fill("hello from e2e");
  await page.getByRole("button", { name: "Send" }).click();

  await expect(page.locator("#chatMessages")).toContainText("hello from e2e");
  await expect(page.locator("#chatMessages")).toContainText(sessionID.slice(0, 8));
});

test("workspace switches chat to terminal and back without growing instances forever", async ({ page }) => {
  await page.goto("/?view=chat");
  await waitForWorkspaceReady(page);

  const sessionID = await createSession(page);
  await page.locator("#chatInput").fill("switch me");
  await page.getByRole("button", { name: "Send" }).click();
  await expect(page.locator("#chatMessages")).toContainText("[fake-claude");

  await page.locator("#workspaceOpenTerminalBtn").click();
  await expect(page).toHaveURL(/\/\?/);
  await expect(page.locator("#workspaceModeBadge")).toContainText("Chat");
  await page.locator("#workspaceSwitchModeBtn").click();
  await expect(page.locator("#workspaceModeBadge")).toContainText("Terminal");
  await expect(page.locator("#terminal")).toContainText("Resumed session");

  await page.locator("#workspaceOpenChatBtn").click();
  await expect(page).toHaveURL(/\/\?.*view=chat/);
  await expect(page.locator("#workspaceModeBadge")).toContainText("Terminal");
  await page.locator("#workspaceSwitchModeBtn").click();
  await expect(page.locator("#workspaceModeBadge")).toContainText("Chat");

  await page.locator("#chatInput").fill("back again");
  await page.getByRole("button", { name: "Send" }).click();
  await expect(page.locator("#chatMessages")).toContainText("back again");
  await expect(page.locator("#instanceHistoryList > li")).toHaveCount(2);
  await expect(page.locator("#workspaceSessionTitle")).toHaveText(sessionID);
});

test("terminal workspace can attach to a preseeded external Claude session", async ({ page }) => {
  await page.goto("/");
  await waitForWorkspaceReady(page);

  await createSession(page, { sessionID: preseededSessionID });
  await expect(page.locator("#workspaceSessionTitle")).toHaveText(preseededSessionID);
  await expect(page.locator("#workspaceModeBadge")).toContainText("Terminal");
  await expect(page.locator("#terminal")).toContainText("Resumed session");
});

test("/chat redirects to the unified workspace chat view", async ({ page }) => {
  await page.goto("/chat?server_id=srv-e2e");
  await expect(page).toHaveURL(/\/\?.*server_id=srv-e2e.*view=chat/);
});
