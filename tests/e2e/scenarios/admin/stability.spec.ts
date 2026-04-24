import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

const ADMIN_PAGES = [
  "/admin",
  "/admin/libraries",
  "/admin/users",
  "/admin/plugins",
  "/admin/webhooks",
  "/admin/feeds",
  "/admin/activity",
  "/admin/sessions",
  "/admin/analytics",
  "/admin/system-health",
  "/admin/server-config",
  "/admin/remote-access",
  "/admin/transcode-cache",
  "/admin/api-keys",
  "/admin/backup",
];

for (const path of ADMIN_PAGES) {
  test.describe(`Stability: ${path}`, () => {
    test("no JS errors", async ({ page }) => {
      const { jsErrors } = await collectErrors(page, async () => {
        await goto(page, path);
        await page.waitForTimeout(1_500);
      });
      expect(jsErrors, `JS errors on ${path}: ${jsErrors.join("; ")}`).toHaveLength(0);
    });

    test("no 5xx API responses", async ({ page }) => {
      const { serverErrors } = await collectErrors(page, async () => {
        await goto(page, path);
        await page.waitForTimeout(1_500);
      });
      expect(serverErrors, `5xx on ${path}: ${serverErrors.join("; ")}`).toHaveLength(0);
    });

    test("has visible heading", async ({ page }) => {
      await goto(page, path);
      await page.waitForTimeout(1_500);
      const heading = page.locator("h1, h2").first();
      await expect(heading).toBeVisible({ timeout: 5_000 });
    });

    test("page is not blank", async ({ page }) => {
      await goto(page, path);
      await page.waitForTimeout(1_500);
      const text = await page.locator("body").textContent();
      expect((text ?? "").trim().length).toBeGreaterThan(10);
    });
  });
}
