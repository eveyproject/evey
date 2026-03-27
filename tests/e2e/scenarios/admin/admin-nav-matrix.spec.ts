import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

/**
 * Admin nav matrix: from each admin page, click every nav link, verify no crash
 */

const ADMIN_PAGES = [
  "/admin", "/admin/libraries", "/admin/users", "/admin/plugins",
  "/admin/webhooks", "/admin/metadata", "/admin/feeds", "/admin/audit",
  "/admin/activity", "/admin/sessions", "/admin/analytics",
  "/admin/system-health", "/admin/server-config", "/admin/logs",
];

// Click each admin nav link from the dashboard
for (const target of ADMIN_PAGES) {
  const name = target.replace("/admin/", "").replace("/admin", "dashboard");
  test(`admin nav → ${name} — loads`, async ({ page }) => {
    await goto(page, "/admin");
    await page.waitForTimeout(1_000);
    const link = page.locator(`a[href="${target}"], a[href*="${name}"]`).first();
    if (await link.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await link.click();
      await page.waitForTimeout(1_500);
      const len = await page.evaluate(() => document.body.innerText.length);
      expect(len).toBeGreaterThan(10);
    }
  });
}

// From each admin page, click "back to app" link
for (const from of ADMIN_PAGES) {
  test(`${from.replace("/admin/", "")} → back to app`, async ({ page }) => {
    await goto(page, from);
    await page.waitForTimeout(1_000);
    const backLink = page.locator("a", { hasText: /back.*app|retour/i }).first();
    if (await backLink.isVisible({ timeout: 2_000 }).catch(() => false)) {
      const { jsErrors } = await collectErrors(page, async () => {
        await backLink.click();
        await page.waitForTimeout(1_000);
      });
      expect(jsErrors).toHaveLength(0);
    }
  });
}

// Rapid navigation between admin pages
test("rapid admin page switches — no crash", async ({ page }) => {
  const { jsErrors } = await collectErrors(page, async () => {
    for (const p of ADMIN_PAGES.slice(0, 7)) {
      await goto(page, p);
      await page.waitForTimeout(300);
    }
  });
  expect(jsErrors).toHaveLength(0);
});
