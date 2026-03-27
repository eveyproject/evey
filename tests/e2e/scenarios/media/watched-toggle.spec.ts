import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia, collectErrors } from "../shared/helpers";

test.describe("Media > Watched toggle", () => {
  test("mark watched button exists on detail", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const btn = page.locator('[data-testid="mark-watched-btn"], [data-testid="mark-unwatched-btn"]').first();
    await expect(btn).toBeVisible({ timeout: 5_000 });
  });

  test("clicking mark watched changes button text", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const watchBtn = page.locator('[data-testid="mark-watched-btn"]');
    const unwatchBtn = page.locator('[data-testid="mark-unwatched-btn"]');
    const hasWatch = await watchBtn.isVisible({ timeout: 3_000 }).catch(() => false);
    const hasUnwatch = await unwatchBtn.isVisible({ timeout: 1_000 }).catch(() => false);
    test.skip(!hasWatch && !hasUnwatch, "No watched button");

    if (hasWatch) {
      await watchBtn.click();
      await page.waitForTimeout(1_500);
      await expect(unwatchBtn).toBeVisible({ timeout: 3_000 });
    } else {
      await unwatchBtn.click();
      await page.waitForTimeout(1_500);
      await expect(watchBtn).toBeVisible({ timeout: 3_000 });
    }
  });

  test("mark watched has no 5xx", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      const id = await gotoRandomMedia(page);
      if (!id) return;
      const btn = page.locator('[data-testid="mark-watched-btn"], [data-testid="mark-unwatched-btn"]').first();
      if (await btn.isVisible({ timeout: 3_000 }).catch(() => false)) {
        await btn.click();
        await page.waitForTimeout(1_500);
      }
    });
    expect(serverErrors).toHaveLength(0);
  });

  test("watched state persists after reload", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const watchBtn = page.locator('[data-testid="mark-watched-btn"]');
    if (await watchBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await watchBtn.click();
      await page.waitForTimeout(1_500);
    }
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    const unwatchVisible = await page.locator('[data-testid="mark-unwatched-btn"]').isVisible({ timeout: 3_000 }).catch(() => false);
    expect(unwatchVisible).toBe(true);

    // cleanup
    await page.locator('[data-testid="mark-unwatched-btn"]').click().catch(() => {});
    await page.waitForTimeout(500);
  });
});
