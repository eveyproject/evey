import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia, goto, collectErrors } from "../shared/helpers";

const watchSel = '[data-testid="mark-watched-btn"]';
const unwatchSel = '[data-testid="mark-unwatched-btn"]';
const anySel = `${watchSel}, ${unwatchSel}`;

async function clickWatchedBtn(page: import("@playwright/test").Page, sel: string) {
  const [response] = await Promise.all([
    page.waitForResponse((r) => r.url().includes("/api/v1/playback/") && r.request().method() === "POST", { timeout: 10_000 }),
    page.locator(sel).click(),
  ]);
  expect(response.status()).toBeLessThan(400);
}

test.describe("Media > Watched (deep)", () => {
  test("mark watched → button changes to 'mark unwatched'", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    const hasWatch = await page.locator(watchSel).isVisible({ timeout: 3_000 }).catch(() => false);
    const hasUnwatch = await page.locator(unwatchSel).isVisible({ timeout: 1_000 }).catch(() => false);
    test.skip(!hasWatch && !hasUnwatch, "No watched button");

    if (hasUnwatch) {
      await clickWatchedBtn(page, unwatchSel);
      await page.waitForSelector(watchSel, { timeout: 5_000 });
    }

    await clickWatchedBtn(page, watchSel);
    await expect(page.locator(unwatchSel)).toBeVisible({ timeout: 10_000 });

    // cleanup
    await clickWatchedBtn(page, unwatchSel);
    await page.waitForTimeout(500);
  });

  test("mark watched → reload → still shows 'mark unwatched'", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    const hasWatch = await page.locator(watchSel).isVisible({ timeout: 3_000 }).catch(() => false);
    if (!hasWatch) {
      const hasUnwatch = await page.locator(unwatchSel).isVisible({ timeout: 1_000 }).catch(() => false);
      if (hasUnwatch) {
        await clickWatchedBtn(page, unwatchSel);
        await page.waitForSelector(watchSel, { timeout: 5_000 });
      } else {
        test.skip(true, "No watched button");
      }
    }

    await clickWatchedBtn(page, watchSel);
    await expect(page.locator(unwatchSel)).toBeVisible({ timeout: 10_000 });

    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 15_000 });
    await expect(page.locator(unwatchSel)).toBeVisible({ timeout: 10_000 });

    // cleanup
    await page.locator(unwatchSel).click().catch(() => {});
    await page.waitForTimeout(500);
  });

  test("mark watched → home page badge visible on that card", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    if (await page.locator(watchSel).isVisible({ timeout: 3_000 }).catch(() => false)) {
      await clickWatchedBtn(page, watchSel);
      await expect(page.locator(unwatchSel)).toBeVisible({ timeout: 10_000 });
    } else {
      test.skip(!(await page.locator(unwatchSel).isVisible({ timeout: 1_000 }).catch(() => false)), "No watched button");
    }

    await goto(page, "/home");
    await page.waitForSelector('a[href*="/media/"]', { timeout: 10_000 }).catch(() => {});

    const card = page.locator(`a[href*='/media/${id}']`).first();
    const cardVisible = await card.isVisible().catch(() => false);
    if (cardVisible) {
      await expect(card.locator('[data-testid="watched-badge"]')).toBeVisible({ timeout: 5_000 });
    }

    // cleanup
    await goto(page, `/media/${id}`);
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 15_000 });
    await page.locator(unwatchSel).click().catch(() => {});
    await page.waitForTimeout(500);
  });

  test("mark unwatched → reload → shows 'mark watched' again", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    if (await page.locator(watchSel).isVisible({ timeout: 3_000 }).catch(() => false)) {
      await clickWatchedBtn(page, watchSel);
      await expect(page.locator(unwatchSel)).toBeVisible({ timeout: 10_000 });
    }
    test.skip(!(await page.locator(unwatchSel).isVisible({ timeout: 2_000 }).catch(() => false)), "Cannot mark watched");

    await clickWatchedBtn(page, unwatchSel);
    await expect(page.locator(watchSel)).toBeVisible({ timeout: 10_000 });

    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 15_000 });
    await expect(page.locator(watchSel)).toBeVisible({ timeout: 10_000 });
  });

  test("mark watched no 5xx, no JS errors", async ({ page }) => {
    const { jsErrors, serverErrors } = await collectErrors(page, async () => {
      const id = await gotoRandomMedia(page);
      if (!id) return;
      const btn = page.locator(anySel).first();
      if (await btn.isVisible({ timeout: 3_000 }).catch(() => false)) {
        await btn.click();
        await page.waitForTimeout(1_500);
        const btn2 = page.locator(anySel).first();
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
