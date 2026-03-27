import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia, goto, collectErrors, apiGet } from "../shared/helpers";

test.describe("Media > Favorite (deep)", () => {
  test("toggle favorite → verify API state via GET", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    const btn = page.locator('[data-testid="favorite-btn"], [data-testid="favorite-btn-active"]').first();
    await expect(btn).toBeVisible({ timeout: 5_000 });

    const wasFav = await page.locator('[data-testid="favorite-btn-active"]').isVisible().catch(() => false);
    await btn.click();
    await page.waitForTimeout(1_500);

    const res = await apiGet(page, `/api/v1/favorites/${id}`);
    expect(res.status).toBeLessThan(500);
    const isFavAPI = res.body?.isFavorite ?? false;
    expect(isFavAPI).toBe(!wasFav);

    // cleanup: toggle back
    await page.locator('[data-testid="favorite-btn"], [data-testid="favorite-btn-active"]').first().click();
    await page.waitForTimeout(500);
  });

  test("toggle favorite → reload → button state matches", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    const btn = page.locator('[data-testid="favorite-btn"], [data-testid="favorite-btn-active"]').first();
    await expect(btn).toBeVisible({ timeout: 5_000 });

    // ensure favorited
    const wasFav = await page.locator('[data-testid="favorite-btn-active"]').isVisible().catch(() => false);
    if (!wasFav) {
      await btn.click();
      await page.waitForTimeout(1_000);
    }

    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    const stillFav = await page.locator('[data-testid="favorite-btn-active"]').isVisible({ timeout: 5_000 }).catch(() => false);
    expect(stillFav).toBe(true);

    // cleanup
    await page.locator('[data-testid="favorite-btn-active"]').first().click();
    await page.waitForTimeout(500);
  });

  test("favorite → /favorites lists it → unfavorite → /favorites removes it", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    const btn = page.locator('[data-testid="favorite-btn"], [data-testid="favorite-btn-active"]').first();
    await expect(btn).toBeVisible({ timeout: 5_000 });

    // ensure favorited
    if (!(await page.locator('[data-testid="favorite-btn-active"]').isVisible().catch(() => false))) {
      await btn.click();
      await page.waitForTimeout(1_000);
    }

    // go to /favorites → media listed
    await goto(page, "/favorites");
    await page.waitForSelector('h1:has-text("Favorites"), [data-testid="page-title"]', { timeout: 10_000 }).catch(() => {});
    const cardLink = page.locator(`a[href*='/media/${id}']`);
    await expect(cardLink.first()).toBeVisible({ timeout: 10_000 });

    // go back and unfavorite
    await goto(page, `/media/${id}`);
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    await page.locator('[data-testid="favorite-btn-active"]').first().click();
    await page.waitForTimeout(1_000);

    // go to /favorites → media gone
    await goto(page, "/favorites");
    await page.waitForTimeout(2_000);
    const movieLink = page.locator(`a[href*='/media/${id}']`);
    const gone = (await movieLink.count()) === 0;
    expect(gone).toBe(true);
  });

  test("favorite toggle no 5xx", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      const id = await gotoRandomMedia(page);
      if (!id) return;
      const btn = page.locator('[data-testid="favorite-btn"], [data-testid="favorite-btn-active"]').first();
      if (await btn.isVisible({ timeout: 3_000 }).catch(() => false)) {
        await btn.click();
        await page.waitForTimeout(1_000);
        // toggle back
        await page.locator('[data-testid="favorite-btn"], [data-testid="favorite-btn-active"]').first().click();
        await page.waitForTimeout(500);
      }
    });
    expect(serverErrors, `5xx: ${serverErrors.join(", ")}`).toHaveLength(0);
  });

  test("double-click favorite rapidly → no duplicate / no crash", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    const btn = page.locator('[data-testid="favorite-btn"], [data-testid="favorite-btn-active"]').first();
    await expect(btn).toBeVisible({ timeout: 5_000 });
    const wasFav = await page.locator('[data-testid="favorite-btn-active"]').isVisible().catch(() => false);

    const { jsErrors, serverErrors } = await collectErrors(page, async () => {
      await btn.dblclick();
      await page.waitForTimeout(1_500);
    });

    expect(jsErrors).toHaveLength(0);
    expect(serverErrors).toHaveLength(0);

    // state should be stable (either same or toggled, but no crash)
    const finalBtn = page.locator('[data-testid="favorite-btn"], [data-testid="favorite-btn-active"]').first();
    await expect(finalBtn).toBeVisible({ timeout: 3_000 });

    // cleanup: restore original state
    const isFav = await page.locator('[data-testid="favorite-btn-active"]').isVisible().catch(() => false);
    if (isFav !== wasFav) {
      await page.locator('[data-testid="favorite-btn"], [data-testid="favorite-btn-active"]').first().click();
      await page.waitForTimeout(500);
    }
  });
});
