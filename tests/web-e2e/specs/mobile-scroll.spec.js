/**
 * mobile-scroll.spec.js
 *
 * Exercises markdown rendering on mobile (code blocks, tables, and images)
 * using cc-chat-echo as backend.
 */

const { test, expect } = require("@playwright/test");
const path = require("node:path");

const repoRoot = process.cwd();
const uiToken = process.env.CC_WEB_E2E_UI_TOKEN || "ui-e2e-token";

// ── helpers ────────────────────────────────────────────────────────────────

async function primeToken(page) {
  await page.addInitScript((token) => {
    localStorage.setItem("ui_token", token);
  }, uiToken);
}

async function waitForWorkspaceReady(page) {
  await expect(page.locator("#wsStatus")).toContainText("connected");
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

async function createSession(page) {
  await openLeftDrawer(page);
  const toolsSummary = page.locator("#workspaceToolsDetails summary");
  const toolsDetails = page.locator("#workspaceToolsDetails");
  if (!(await toolsDetails.evaluate((el) => el.hasAttribute("open")))) {
    await toolsSummary.click();
  }
  await page.getByLabel("cwd").fill(repoRoot);
  await page.getByRole("button", { name: "Create" }).click();
  await expect(page.locator("#workspaceSessionTitle")).not.toHaveText("No session selected");
  await closeLeftDrawer(page);
}

async function sendMessage(page, text) {
  await page.locator("#chatInput").fill(text);
  await page.getByRole("button", { name: "Send" }).click();
}

// ── setup ──────────────────────────────────────────────────────────────────

test.beforeEach(async ({ page }) => {
  await primeToken(page);
  await page.goto("/?view=chat");
  await waitForWorkspaceReady(page);
  await createSession(page);
});

// ── tests ──────────────────────────────────────────────────────────────────

test("chat-bubble code block is horizontally scrollable on mobile", async ({ page }) => {
  // A Python line that is intentionally very long so it overflows the bubble
  // width on a 390px mobile screen.
  const longLine =
    "x = 'this_is_a_very_long_variable_name_that_must_not_wrap_on_mobile' * 8  # overflow";

  // cc-chat-echo will echo back: "[echo] <our message>".
  // The assertion targets the user bubble code block to avoid echo formatting quirks.
  const userMsg = `\n\`\`\`python\n${longLine}\n\`\`\``;
  await sendMessage(page, userMsg);

  // Pick the actual user code block (assistant echo may produce an empty fenced block).
  const codeBlock = page.locator(".chat-bubble.user .chat-md-code").filter({
    hasText: "must_not_wrap_on_mobile",
  }).first();
  await expect(codeBlock).toBeVisible({ timeout: 15_000 });

  // Assert we can actually scroll it horizontally.
  const scrollLeftBefore = await codeBlock.evaluate((el) => el.scrollLeft);
  expect(scrollLeftBefore).toBe(0);

  await codeBlock.evaluate((el) => {
    el.scrollBy({ left: 200, behavior: "auto" });
  });

  const scrollLeftAfter = await codeBlock.evaluate((el) => el.scrollLeft);
  expect(scrollLeftAfter).toBeGreaterThan(0);
});

test("code block word-break does not wrap long identifiers", async ({ page }) => {
  // Verify that code inside a code block does NOT get broken mid-word.
  // If word-break: break-all is incorrectly applied, the code content
  // would wrap and scrollWidth would equal clientWidth (no overflow needed).
  const longIdentifier = "very_long_python_identifier_" + "x".repeat(80) + " = 42";
  const userMsg = `\n\`\`\`python\n${longIdentifier}\n\`\`\``;
  await sendMessage(page, userMsg);

  const codeBlock = page.locator(".chat-bubble.user .chat-md-code").filter({
    hasText: "very_long_python_identifier_",
  }).first();
  await expect(codeBlock).toBeVisible({ timeout: 15_000 });

  // code element inside should not have word-break forced
  const codeEl = codeBlock.locator("code");
  const wordBreak = await codeEl.evaluate((el) =>
    window.getComputedStyle(el).wordBreak
  );
  // Should be "normal" (or "keep-all"), NOT "break-all"
  expect(wordBreak).not.toBe("break-all");
});

test("markdown table renders with expected structure on mobile", async ({ page }) => {
  const tableMarkdown = [
    "| Name | Status | Detail |",
    "| --- | --- | --- |",
    "| Alpha | ok | row-1 |",
    "| Beta | pending | row-2 |",
  ].join("\n");
  await sendMessage(page, tableMarkdown);

  const tableWrap = page.locator(".chat-bubble.user .chat-md-table-wrap").filter({
    hasText: "Alpha",
  }).first();
  await expect(tableWrap).toBeVisible({ timeout: 15_000 });

  const table = tableWrap.locator("table");
  await expect(table).toBeVisible();
  await expect(tableWrap.locator("thead th")).toHaveCount(3);
  await expect(tableWrap.locator("tbody tr")).toHaveCount(2);

  const overflowX = await tableWrap.evaluate((el) => window.getComputedStyle(el).overflowX);
  expect(overflowX === "auto" || overflowX === "scroll").toBeTruthy();
});

test("markdown image renders from data URL on mobile", async ({ page }) => {
  const tinyPngBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO7Y5WQAAAAASUVORK5CYII=";
  const userMsg = `![tiny](data:image/png;base64,${tinyPngBase64})`;
  await sendMessage(page, userMsg);

  const img = page.locator(".chat-bubble.user .chat-markdown img[alt='tiny']").first();
  await expect(img).toBeVisible({ timeout: 15_000 });
  await expect(img).toHaveAttribute("src", new RegExp("^data:image/png;base64,"));

  const imageLoaded = await img.evaluate((el) => {
    if (!(el instanceof HTMLImageElement)) return false;
    return el.naturalWidth > 0 && el.naturalHeight > 0;
  });
  expect(imageLoaded).toBe(true);
});

test("chat input is visible and not hidden after messages load on mobile", async ({ page }) => {
  // Regression: chat-input-bar was missing flex-shrink:0 and got squished
  // off-screen when chat-messages expanded.
  await sendMessage(page, "hello mobile");

  // Wait for echo response
  await expect(page.locator("#chatMessages")).toContainText("[echo]", { timeout: 15_000 });

  // The Send button must still be visible and in the viewport
  const sendBtn = page.getByRole("button", { name: "Send" });
  await expect(sendBtn).toBeVisible();

  // Double-check it is actually within the viewport bounds
  const btnBox = await sendBtn.boundingBox();
  const viewportSize = page.viewportSize();
  expect(btnBox).not.toBeNull();
  expect(btnBox.y + btnBox.height).toBeLessThanOrEqual(viewportSize.height + 2); // +2px tolerance
});
