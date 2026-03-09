import { test as base, expect } from "@playwright/test";

type AuthFixtures = {
  authedPage: ReturnType<typeof base.extend> extends infer T ? T : never;
};

export const test = base.extend({
  page: async ({ page }, use) => {
    await page.goto("/login", { waitUntil: "networkidle" });
    await page.fill("#username", process.env.EVEY_USER ?? "gravity");
    await page.fill("#password", process.env.EVEY_PASS ?? "REDACTED");
    await page.click('button[type="submit"]');
    await page.waitForURL("**/home", { timeout: 15_000 });
    await use(page);
  },
});

export { expect };
