const { test, expect } = require("@playwright/test");
const crypto = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");

const repoRoot = process.cwd();
const uiToken = process.env.CC_WEB_E2E_UI_TOKEN || "ui-e2e-token";
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
  await page.route("https://cdn.jsdelivr.net/npm/xterm-addon-unicode11/lib/xterm-addon-unicode11.js", async (route) => {
    await route.fulfill({ status: 200, contentType: "application/javascript", body: "window.Unicode11Addon = window.Unicode11Addon || { Unicode11Addon: class {} };" });
  });
}

async function primeToken(page) {
  await page.addInitScript((token) => {
    localStorage.setItem("ui_token", token);
  }, uiToken);
}

async function waitForWorkspaceReady(page) {
  await page.locator("#tokenInput").fill(uiToken);
  await page.locator("#saveTokenBtn").click();
  await expect(page.locator("#wsStatus")).toHaveText(/WS:\s*connected/i);

  await expect.poll(async () => {
    const resp = await page.request.get("/api/servers", {
      headers: { Authorization: `Bearer ${uiToken}` },
    });
    if (resp.status() !== 200) return "";
    const body = await resp.json();
    const online = (body.servers || []).some((s) => s.server_id === "srv-e2e" && s.status === "online");
    return online ? "online" : "";
  }, { timeout: 30_000 }).toBe("online");
}

test.beforeEach(async ({ page }) => {
  await installXtermStub(page);
  await primeToken(page);
});

test("workspace shows notification toast and notification list item", async ({ page }, testInfo) => {
  await page.goto("/");
  await waitForWorkspaceReady(page);

  const fixedSessionID = crypto.randomUUID();
  const createResp = await page.request.post("/api/sessions", {
    headers: {
      Authorization: `Bearer ${uiToken}`,
      "Content-Type": "application/json",
    },
    data: {
      server_id: "srv-e2e",
      session_id: fixedSessionID,
      cwd: repoRoot,
      session_type: "pty",
      env: {},
      cols: 120,
      rows: 30,
    },
  });
  expect(createResp.status()).toBe(201);

  const uniqueText = `notification-e2e-${Date.now()}`;
  const title = "E2E Notification";

  const resp = await page.request.post("/api/notifications", {
    headers: {
      Authorization: `Bearer ${uiToken}`,
      "Content-Type": "application/json",
    },
    data: {
      title,
      message: uniqueText,
      level: "success",
      source: "web-e2e",
      session_id: fixedSessionID,
    },
  });
  expect(resp.status()).toBe(201);

  await page.reload();
  await waitForWorkspaceReady(page);

  await expect(page.locator("#noticeList")).toContainText(uniqueText);
  await expect(page.locator("#noticeList")).toContainText(title);
  await expect(page.locator("#notificationToasts")).toContainText(uniqueText);
  await expect.poll(async () => await page.locator("#notificationToasts .notification-toast").count()).toBeGreaterThan(0);
  await expect.poll(async () => Number((await page.locator("#noticeCount").innerText()).trim() || "0")).toBeGreaterThan(0);

  await page.screenshot({
    path: testInfo.outputPath("notification-visible.png"),
    fullPage: true,
    animations: "disabled",
  });
});
