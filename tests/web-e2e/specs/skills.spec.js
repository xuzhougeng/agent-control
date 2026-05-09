const { test, expect } = require("@playwright/test");

const uiToken = process.env.CC_WEB_E2E_UI_TOKEN || "ui-e2e-token";

async function primeToken(page) {
  await page.addInitScript((token) => {
    localStorage.setItem("ui_token", token);
  }, uiToken);
}

test.describe("Skills tab", () => {
  test("seeds, lists, selects, and shows install button", async ({ page, baseURL }) => {
    // Pre-seed the registry.
    const publishResp = await page.request.post(`${baseURL}/api/registry/skills`, {
      headers: {
        Authorization: `Bearer ${uiToken}`,
        "Content-Type": "application/json",
      },
      data: {
        name: "demo-skill",
        description: "Demo skill for e2e",
        prompt: "hello world",
        tools: ["bash"],
        examples: ["ping"],
      },
    });
    expect(publishResp.status(), await publishResp.text()).toBe(201);

    await primeToken(page);
    await page.goto("/skills.html");
    await expect(page.getByRole("heading", { name: "Skills", level: 2 })).toBeVisible();

    // List should show our seeded skill.
    const firstRow = page.locator("ul#skills-list li").first();
    await expect(firstRow).toContainText("demo-skill");

    // Click it; detail panel renders.
    await firstRow.click();
    const detailHeading = page.locator("section#skills-detail h3");
    await expect(detailHeading).toContainText("demo-skill");
    await expect(page.locator("#skills-install-btn")).toBeVisible();
    await expect(page.locator(".skills-tools .tag")).toContainText("bash");
  });
});
