import { defineConfig } from "@playwright/test";
import path from "node:path";
import { fileURLToPath } from "node:url";

const port = Number(process.env.CC_WEB_E2E_PORT || 18110);
const configDir = path.dirname(fileURLToPath(import.meta.url));
const harnessPath = path.join(configDir, "run-harness.sh");
const screenshotMode = process.env.CC_WEB_E2E_SCREENSHOT || "on";

export default defineConfig({
  testDir: "./specs",
  timeout: 90_000,
  expect: {
    timeout: 10_000,
  },
  fullyParallel: false,
  retries: process.env.CI ? 1 : 0,
  reporter: [["list"]],
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    headless: true,
    trace: "retain-on-failure",
    screenshot: {
      mode: screenshotMode,
      fullPage: true,
    },
  },
  webServer: {
    command: `bash ${JSON.stringify(harnessPath)}`,
    url: `http://127.0.0.1:${port}`,
    timeout: 120_000,
    reuseExistingServer: !process.env.CI,
    stdout: "pipe",
    stderr: "pipe",
  },
});
