import { defineConfig } from "@playwright/test";
import { STORAGE_STATE } from "./global-setup";

export default defineConfig({
  globalSetup: "./global-setup.ts",
  globalTeardown: "./global-teardown.ts",
  testDir: ".",
  testMatch: "**/*.spec.ts",
  timeout: 60_000,
  expect: { timeout: 10_000 },
  retries: 0,
  reporter: [["html", { open: "never" }]],
  use: {
    baseURL: process.env.BASE_URL ?? "https://evey.test",
    ignoreHTTPSErrors: true,
    // Reuse the single login from global-setup so tests don't each hammer
    // /auth/login (LoginRL 10/min/IP → 429→500 avalanche that produced mass
    // false failures on full-suite runs). Specs needing a logged-out context
    // override with test.use({ storageState: { cookies: [], origins: [] } }).
    storageState: STORAGE_STATE,
    screenshot: "only-on-failure",
    video: "retain-on-failure",
    trace: "retain-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: { browserName: "chromium" },
    },
    {
      name: "firefox",
      use: { browserName: "firefox" },
    },
  ],
});
