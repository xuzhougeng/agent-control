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
  await openLeftDrawer(page);
  await expect(page.locator("#serversList")).toContainText("srv-e2e");
  await closeLeftDrawer(page);
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

async function dragDrawerVertically(page, handleSelector, deltaY) {
  await page.locator(handleSelector).evaluate((el, moveBy) => {
    const rect = el.getBoundingClientRect();
    const clientX = rect.left + (rect.width / 2);
    const clientY = rect.top + (rect.height / 2);
    const pointerId = 1;
    el.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true, button: 0, pointerId, clientX, clientY }));
    window.dispatchEvent(new PointerEvent("pointermove", { bubbles: true, button: 0, pointerId, clientX, clientY: clientY + moveBy }));
    window.dispatchEvent(new PointerEvent("pointerup", { bubbles: true, button: 0, pointerId, clientX, clientY: clientY + moveBy }));
  }, deltaY);
}

async function getDrawerTop(page, selector) {
  return page.locator(selector).evaluate((el) => parseFloat(window.getComputedStyle(el).top || "0"));
}

async function createSession(page, { cwd, sessionID } = {}) {
  const root = cwd || repoRoot;
  await openLeftDrawer(page);
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
  await expect(page.locator("#chatMessages")).toContainText("result: status=ok");
  await expect(page.locator("#workspaceSessionTitle")).toHaveText(sessionID);
});

test("workspace switches chat to terminal and back without growing instances forever", async ({ page }) => {
  await page.goto("/?view=chat");
  await waitForWorkspaceReady(page);

  const sessionID = await createSession(page);
  await page.locator("#chatInput").fill("switch me");
  await page.getByRole("button", { name: "Send" }).click();
  await expect(page.locator("#chatMessages")).toContainText("init: model=fake-claude");

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
  await openRightDrawer(page);
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

test("workspace drawers can be dragged vertically", async ({ page }) => {
  await page.goto("/");
  await waitForWorkspaceReady(page);

  await openLeftDrawer(page);
  const leftTopBefore = await getDrawerTop(page, "#sidebar");
  await dragDrawerVertically(page, '[data-drawer-drag="left"]', 96);
  const leftTopAfter = await getDrawerTop(page, "#sidebar");
  expect(leftTopAfter).toBeGreaterThan(leftTopBefore + 40);

  await openRightDrawer(page);
  const rightTopBefore = await getDrawerTop(page, "#contextSidebar");
  await dragDrawerVertically(page, '[data-drawer-drag="right"]', 72);
  const rightTopAfter = await getDrawerTop(page, "#contextSidebar");
  expect(rightTopAfter).toBeGreaterThan(rightTopBefore + 30);
});

test("mobile workspace uses drawer toggles for sessions and context", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/");
  await waitForWorkspaceReady(page);

  const leftBtn = page.locator("#leftDrawerToggleBtn");
  const rightBtn = page.locator("#rightDrawerToggleBtn");
  const backdrop = page.locator("#sidebarBackdrop");

  await expect(leftBtn).toBeVisible();
  await expect(rightBtn).toBeVisible();

  await rightBtn.click();
  await expect(rightBtn).toHaveAttribute("aria-expanded", "true");
  await expect.poll(async () => page.evaluate(() => document.body.classList.contains("right-drawer-open"))).toBeTruthy();
  await expect.poll(async () => page.evaluate(() => document.body.classList.contains("left-drawer-open"))).toBeFalsy();

  await leftBtn.click();
  await expect(leftBtn).toHaveAttribute("aria-expanded", "true");
  await expect.poll(async () => page.evaluate(() => document.body.classList.contains("left-drawer-open"))).toBeTruthy();
  await expect.poll(async () => page.evaluate(() => document.body.classList.contains("right-drawer-open"))).toBeFalsy();

  await backdrop.click({ force: true });
  await expect.poll(async () => page.evaluate(() => ({
    left: document.body.classList.contains("left-drawer-open"),
    right: document.body.classList.contains("right-drawer-open"),
    mobile: document.body.classList.contains("sidebar-open"),
  }))).toEqual({ left: false, right: false, mobile: false });
});

test("terminal copy event writes selected text to clipboard", async ({ page }) => {
  await page.addInitScript(() => {
    const clipboard = {
      async writeText(text) {
        window.__ccCopiedText = String(text || "");
      },
    };
    Object.defineProperty(window.navigator, "clipboard", {
      configurable: true,
      value: clipboard,
    });
  });

  await page.goto("/");
  await waitForWorkspaceReady(page);

  await page.evaluate(() => {
    const terminal = document.getElementById("terminal");
    if (!terminal) return;
    terminal.textContent = "copy-target-line";
    const selection = window.getSelection();
    if (!selection) return;
    const range = document.createRange();
    range.selectNodeContents(terminal);
    selection.removeAllRanges();
    selection.addRange(range);
    terminal.dispatchEvent(new Event("copy", { bubbles: true, cancelable: true }));
  });

  await expect.poll(async () => page.evaluate(() => window.__ccCopiedText || "")).toContain("copy-target-line");
});

test("/chat redirects to the unified workspace chat view", async ({ page }) => {
  await page.goto("/chat?server_id=srv-e2e");
  await expect(page).toHaveURL(/\/\?.*server_id=srv-e2e.*view=chat/);
});
