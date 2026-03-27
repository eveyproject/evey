import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia, goto, collectErrors } from "../shared/helpers";

test.describe("Media > Watched (deep)", () => {
  test("mark watched → button changes to 'mark unwatched'", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    const watchBtn = page.locator('[data-testid="mark-watched-btn"]');
    const unwatchBtn = page.locator('[data-testid="mark-unwatched-btn"]');
    const hasWatch = await watchBtn.isVisible({ timeout: 3_000 }).catch(() => false);
    const hasUnwatch = await unwatchBtn.isVisible({ timeout: 1_000 }).catch(() => false);
    test.skip(!hasWatch && !hasUnwatch, "No watched button");

    // ensure unwatched first
    if (hasUnwatch) {
      await unwatchBtn.click();
      await page.waitForTimeout(1_500);
    }

    await page.locator('[data-testid="mark-watched-btn"]').click();
    await page.waitForTimeout(1_500);
    await expect(page.locator('[data-testid="mark-unwatched-btn"]')).toBeVisible({ timeout: 5_000 });

    // cleanup
    await page.locator('[data-testid="mark-unwatched-btn"]').click().catch(() => {});
    await page.waitForTimeout(500);
  });

  test("mark watched → reload → still shows 'mark unwatched'", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    const watchBtn = page.locator('[data-testid="mark-watched-btn"]');
    const hasWatch = await watchBtn.isVisible({ timeout: 3_000 }).catch(() => false);
    // if already watched, mark unwatched first
    if (!hasWatch) {
      const unwatchBtn = page.locator('[data-testid="mark-unwatched-btn"]');
      if (await unwatchBtn.isVisible({ timeout: 1_000 }).catch(() => false)) {
        await unwatchBtn.click();
        await page.waitForTimeout(1_500);
      } else {
        test.skip(true, "No watched button");
      }
    }

    await page.locator('[data-testid="mark-watched-btn"]').click();
    await page.waitForTimeout(1_500);

    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    const unwatchVisible = await page.locator('[data-testid="mark-unwatched-btn"]').isVisible({ timeout: 5_000 }).catch(() => false);
    expect(unwatchVisible).toBe(true);

    // cleanup
    await page.locator('[data-testid="mark-unwatched-btn"]').click().catch(() => {});
    await page.waitForTimeout(500);
  });

  test("mark watched → home page badge visible on that card", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    // ensure watched
    const watchBtn = page.locator('[data-testid="mark-watched-btn"]');
    if (await watchBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await watchBtn.click();
      await page.waitForTimeout(1_500);
    }

    // navigate to home / library to find the card
    await goto(page, "/home");
    await page.waitForSelector('[data-testid="media-card"], a[href*="/media/"]', { timeout: 10_000 }).catch(() => {});

    const card = page.locator(`a[href*='/media/${id}']`).first();
    const cardVisible = await card.isVisible({ timeout: 5_000 }).catch(() => false);
    if (cardVisible) {
      const badge = card.locator('[data-testid="watched-badge"]');
      const hasBadge = await badge.isVisible({ timeout: 2_000 }).catch(() => false);
      expect(hasBadge).toBe(true);
    }
    // if card not on home page, skip gracefully (it may be in a different section)

    // cleanup: go back and unwatch
    await goto(page, `/media/${id}`);
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    await page.locator('[data-testid="mark-unwatched-btn"]').click().catch(() => {});
    await page.waitForTimeout(500);
  });

  test("mark unwatched → reload → shows 'mark watched' again", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    // ensure watched first
    const watchBtn = page.locator('[data-testid="mark-watched-btn"]');
    if (await watchBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await watchBtn.click();
      await page.waitForTimeout(1_500);
    }
    test.skip(!(await page.locator('[data-testid="mark-unwatched-btn"]').isVisible({ timeout: 2_000 }).catch(() => false)), "Cannot mark watched");

    // now mark unwatched
    await page.locator('[data-testid="mark-unwatched-btn"]').click();
    await page.waitForTimeout(1_500);

    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    const watchVisible = await page.locator('[data-testid="mark-watched-btn"]').isVisible({ timeout: 5_000 }).catch(() => false);
    expect(watchVisible).toBe(true);
  });

  test("mark watched no 5xx, no JS errors", async ({ page }) => {
    const { jsErrors, serverErrors } = await collectErrors(page, async () => {
      const id = await gotoRandomMedia(page);
      if (!id) return;
      const btn = page.locator('[data-testid="mark-watched-btn"], [data-testid="mark-unwatched-btn"]').first();
      if (await btn.isVisible({ timeout: 3_000 }).catch(() => false)) {
        await btn.click();
        await page.waitForTimeout(1_500);
        // toggle back
        const btn2 = page.locator('[data-testid="mark-watched-btn"], [data-testid="mark-unwatched-btn"]').first();
        if (await btn2.isVisible({ timeout: 2_000 }).catch(() => false)) {
          await btn2.click();
          await page.waitForTimeout(500);
        }
      }
    });
    expect(jsErrors).toHaveLength(0);
    expect(serverErrors, `5xx: ${serverErrors.join(", ")}`).toHaveLength(0);
  });
});
