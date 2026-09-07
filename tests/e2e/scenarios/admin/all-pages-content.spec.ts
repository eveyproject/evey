import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

/**
 * For every admin page: verify it loads, has content, no errors.
 * Each page is its own test so failures are granular.
 */

const ADMIN_PAGES = [
  { path: "/admin", name: "Dashboard" },
  { path: "/admin/libraries", name: "Libraries" },
  { path: "/admin/users", name: "Users" },
  { path: "/admin/plugins", name: "Plugins" },
  { path: "/admin/webhooks", name: "Webhooks" },
  { path: "/admin/feeds", name: "Feeds" },
  { path: "/admin/activity", name: "Activity" },
  { path: "/admin/sessions", name: "Sessions" },
  { path: "/admin/analytics", name: "Analytics" },
  { path: "/admin/system-health", name: "System Health" },
  { path: "/admin/server-config", name: "Server Config" },
  { path: "/admin/remote-access", name: "Remote Access" },
  { path: "/admin/transcode-cache", name: "Transcode Cache" },
  { path: "/admin/api-keys", name: "API Keys" },
  { path: "/admin/backup", name: "Backup" },
  { path: "/admin/media-health", name: "Media Health" },
];

for (const pg of ADMIN_PAGES) {
  test.describe(`Admin ${pg.name}`, () => {
    test("has visible heading", async ({ page }) => {
      await goto(page, pg.path);
      await page.waitForTimeout(2_000);
      const h = page.locator("h1, h2").first();
      await expect(h).toBeVisible({ timeout: 5_000 });
    });

    test("page is not blank", async ({ page }) => {
      await goto(page, pg.path);
      await page.waitForTimeout(2_000);
      const len = await page.evaluate(() => document.body.innerText.length);
      expect(len).toBeGreaterThan(20);
    });

    test("no JS errors", async ({ page }) => {
      const { jsErrors } = await collectErrors(page, async () => {
        await goto(page, pg.path);
        await page.waitForTimeout(2_000);
      });
      expect(jsErrors).toHaveLength(0);
    });

    test("no 5xx API responses", async ({ page }) => {
      const { serverErrors } = await collectErrors(page, async () => {
        await goto(page, pg.path);
        await page.waitForTimeout(2_000);
      });
      expect(serverErrors).toHaveLength(0);
    });

    test("no untranslated i18n keys", async ({ page }) => {
      await goto(page, pg.path);
      await page.waitForTimeout(2_000);
      const body = await page.locator("body").textContent();
      const keys = (body!.match(/\b(?:admin|common|media|auth|errors)\.[a-z]+\.[a-zA-Z]+/g) ?? [])
        .filter(k => !k.includes("http"));
      expect(keys, `Untranslated: ${keys.join(", ")}`).toHaveLength(0);
    });

    test("no [object Object] visible", async ({ page }) => {
      await goto(page, pg.path);
      await page.waitForTimeout(2_000);
      const body = await page.locator("body").textContent();
      expect(body).not.toContain("[object Object]");
    });
  });
}
