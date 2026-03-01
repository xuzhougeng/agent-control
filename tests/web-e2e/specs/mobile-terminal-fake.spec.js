const { test, expect } = require("@playwright/test");
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
    await route.fulfill({
      status: 200,
      contentType: "application/javascript",
      body: "window.FitAddon = window.FitAddon || { FitAddon: class { fit() {} } };",
    });
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
    const backdrop = page.locator("#sidebarBackdrop");
    await backdrop.click({ force: true });
    await expect(btn).toHaveAttribute("aria-expanded", "false");
  }
}

async function waitForWorkspaceReady(page) {
  await expect(page.locator("#wsStatus")).toContainText("connected");
  await openLeftDrawer(page);
  await expect(page.locator("#serversList")).toContainText("srv-e2e");
  await closeLeftDrawer(page);
}

async function createTerminalSession(page) {
  const toolsSummary = page.locator("#workspaceToolsDetails summary");
  const toolsDetails = page.locator("#workspaceToolsDetails");
  if (!(await toolsDetails.evaluate((el) => el.hasAttribute("open")))) {
    await toolsSummary.click();
  }
  await page.getByLabel("cwd").fill(repoRoot);
  await page.getByRole("button", { name: "Create" }).click();
  await expect(page.locator("#workspaceSessionTitle")).not.toHaveText("No session selected");
}

function sanitizeStepLabel(label) {
  return String(label).toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
}

async function captureStep(page, testInfo, label, options = {}) {
  const highlightTerminal = options.highlightTerminal === true;
  if (highlightTerminal) {
    await page.evaluate(() => {
      let style = document.getElementById("__cc_e2e_terminal_shot_style");
      if (!style) {
        style = document.createElement("style");
        style.id = "__cc_e2e_terminal_shot_style";
        style.textContent = `
          #terminal {
            background: #07152f !important;
            color: #f6fbff !important;
          }
          #terminal * {
            color: inherit !important;
          }
        `;
        document.head.appendChild(style);
      }
    });
  }

  await page.screenshot({
    path: testInfo.outputPath(`${sanitizeStepLabel(label)}.png`),
    fullPage: true,
    animations: "disabled",
    timeout: 15_000,
  });

  if (highlightTerminal) {
    await page.evaluate(() => {
      document.getElementById("__cc_e2e_terminal_shot_style")?.remove();
    });
  }
}

async function getTerminalIOState(page) {
  return page.evaluate(() => {
    if (!window.__CC_E2E__ || typeof window.__CC_E2E__.getTerminalIOState !== "function") return null;
    return window.__CC_E2E__.getTerminalIOState();
  });
}

test.beforeEach(async ({ page }) => {
  await installXtermStub(page);
  await primeToken(page);
});

test("mobile terminal works with fake Claude session", async ({ page }, testInfo) => {
  await page.goto("/");
  await captureStep(page, testInfo, "01-workspace-loaded");

  await waitForWorkspaceReady(page);
  await openLeftDrawer(page);
  await captureStep(page, testInfo, "02-workspace-ready-drawer-open");

  await createTerminalSession(page);
  await captureStep(page, testInfo, "03-session-created-drawer-open");
  await closeLeftDrawer(page);
  await captureStep(page, testInfo, "04-session-created-drawer-closed");

  await expect(page.locator("#workspaceModeBadge")).toContainText("Terminal");
  await expect(page.locator("#terminal")).toContainText("Started session", { timeout: 20_000 });
  await captureStep(page, testInfo, "05-terminal-started", { highlightTerminal: true });

  const ioBeforeSend = (await getTerminalIOState(page)) || { termInCount: 0, termOutCount: 0 };
  const sendOK = await page.evaluate(() => window.__CC_E2E__.sendTerminalInput("mobile fake terminal\r"));
  expect(sendOK).toBeTruthy();
  await expect
    .poll(async () => {
      const io = await getTerminalIOState(page);
      return Number(io?.termInCount || 0);
    }, { timeout: 20_000 })
    .toBeGreaterThan(Number(ioBeforeSend.termInCount || 0));
  await captureStep(page, testInfo, "06-terminal-input-observed", { highlightTerminal: true });

  await expect(page.locator("#terminal")).toContainText("echo: mobile fake terminal", { timeout: 20_000 });
  await expect
    .poll(async () => {
      const io = await getTerminalIOState(page);
      return Number(io?.termOutCount || 0);
    }, { timeout: 20_000 })
    .toBeGreaterThan(Number(ioBeforeSend.termOutCount || 0));
  await captureStep(page, testInfo, "07-terminal-echo-received", { highlightTerminal: true });
});
