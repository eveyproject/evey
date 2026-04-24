import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

test.describe("Admin > Webhooks", () => {
  test("page loads with heading", async ({ page }) => {
    await goto(page, "/admin/webhooks");
    const heading = page.locator("h1, h2").first();
    await expect(heading).toBeVisible({ timeout: 5_000 });
  });

  test("create button exists", async ({ page }) => {
    await goto(page, "/admin/webhooks");
    await expect(page.locator('[data-testid="add-webhook-btn"]')).toBeVisible({ timeout: 5_000 });
  });

  test("create button opens modal/form", async ({ page }) => {
    await goto(page, "/admin/webhooks");
    await page.locator('[data-testid="add-webhook-btn"]').click();
    await page.waitForTimeout(500);
    const inputs = page.locator("input");
    expect(await inputs.count()).toBeGreaterThan(0);
  });

  test("no 5xx", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, "/admin/webhooks");
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);
  });

  test("no JS errors", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await goto(page, "/admin/webhooks");
      await page.waitForTimeout(2_000);
    });
    expect(jsErrors).toHaveLength(0);
  });
});
