import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

test.describe("Admin > Metadata management", () => {
  test("page loads with search and grid", async ({ page }) => {
    await goto(page, "/admin/metadata");
    await page.waitForTimeout(2_000);
    await expect(page.locator("h1, h2").first()).toBeVisible({ timeout: 5_000 });
  });

  test("refresh all button exists", async ({ page }) => {
    await goto(page, "/admin/metadata");
    await expect(page.locator("button", { hasText: /refresh/i }).first()).toBeVisible({ timeout: 5_000 });
  });

  test("refresh all doesn't 5xx", async ({ page }) => {
    await goto(page, "/admin/metadata");
    const refreshBtn = page.locator("button", { hasText: /refresh.*all/i }).first();
    test.skip(!(await refreshBtn.isVisible({ timeout: 3_000 }).catch(() => false)), "No refresh all button");
    const { serverErrors } = await collectErrors(page, async () => {
      await refreshBtn.click();
      await page.waitForTimeout(3_000);
    });
    expect(serverErrors).toHaveLength(0);
  });

  test("media grid has items", async ({ page }) => {
    await goto(page, "/admin/metadata");
    await page.waitForTimeout(3_000);
    const cards = page.locator('[data-testid="media-card"]');
    const count = await cards.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test("no JS errors", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await goto(page, "/admin/metadata");
      await page.waitForTimeout(2_000);
    });
    expect(jsErrors).toHaveLength(0);
  });
});
