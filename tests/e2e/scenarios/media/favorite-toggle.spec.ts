import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia, collectErrors } from "../shared/helpers";

test.describe("Media > Favorite toggle", () => {
  test("favorite button exists on detail page", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const btn = page.locator('[data-testid="favorite-btn"], [data-testid="favorite-btn-active"]').first();
    await expect(btn).toBeVisible({ timeout: 5_000 });
  });

  test("clicking favorite toggles state", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const btn = page.locator('[data-testid="favorite-btn"], [data-testid="favorite-btn-active"]').first();
    await expect(btn).toBeVisible({ timeout: 5_000 });
    const wasFav = await page.locator('[data-testid="favorite-btn-active"]').isVisible().catch(() => false);
    await btn.click();
    await page.waitForTimeout(1_000);
    const isFav = await page.locator('[data-testid="favorite-btn-active"]').isVisible().catch(() => false);
    expect(isFav).not.toBe(wasFav);
  });

  test("favorite toggle has no JS errors", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      const id = await gotoRandomMedia(page);
      if (!id) return;
      const btn = page.locator('[data-testid="favorite-btn"], [data-testid="favorite-btn-active"]').first();
      if (await btn.isVisible({ timeout: 3_000 }).catch(() => false)) {
        await btn.click();
        await page.waitForTimeout(1_000);
      }
    });
    expect(jsErrors).toHaveLength(0);
  });

  test("favorite toggle has no 5xx", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      const id = await gotoRandomMedia(page);
      if (!id) return;
      const btn = page.locator('[data-testid="favorite-btn"], [data-testid="favorite-btn-active"]').first();
      if (await btn.isVisible({ timeout: 3_000 }).catch(() => false)) {
        await btn.click();
        await page.waitForTimeout(1_000);
      }
    });
    expect(serverErrors).toHaveLength(0);
  });

  test("favorite persists after reload", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const btn = page.locator('[data-testid="favorite-btn"], [data-testid="favorite-btn-active"]').first();
    test.skip(!(await btn.isVisible({ timeout: 3_000 }).catch(() => false)), "No favorite button");

    // ensure it's favorited
    const wasFav = await page.locator('[data-testid="favorite-btn-active"]').isVisible().catch(() => false);
    if (!wasFav) {
      await btn.click();
      await page.waitForTimeout(1_000);
    }
    // reload
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    const stillFav = await page.locator('[data-testid="favorite-btn-active"]').isVisible({ timeout: 3_000 }).catch(() => false);
    expect(stillFav).toBe(true);

    // cleanup: unfavorite
    await page.locator('[data-testid="favorite-btn-active"]').first().click();
    await page.waitForTimeout(500);
  });
});
