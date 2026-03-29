import { test, expect } from "../../fixtures/auth";
import { goto, gotoRandomMedia, collectErrors } from "../shared/helpers";

const watchSel = '[data-testid="mark-watched-btn"]';
const unwatchSel = '[data-testid="mark-unwatched-btn"]';

test.describe("Watch History", () => {
  test("history page loads", async ({ page }) => {
    await goto(page, "/profile/history");
    await page.waitForTimeout(2_000);
    const content = page.locator('[data-testid="history-list"], [data-testid="history-empty"]');
    await expect(content.first()).toBeVisible({ timeout: 10_000 });
  });

  test("mark watched → appears in history", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    const hasWatch = await page.locator(watchSel).isVisible({ timeout: 3_000 }).catch(() => false);
    if (hasWatch) {
      await Promise.all([
        page.waitForResponse((r) => r.url().includes("/api/v1/playback/") && r.request().method() === "POST", { timeout: 10_000 }),
        page.locator(watchSel).click(),
      ]);
      await expect(page.locator(unwatchSel)).toBeVisible({ timeout: 10_000 });
    } else {
      test.skip(!(await page.locator(unwatchSel).isVisible({ timeout: 1_000 }).catch(() => false)), "No watched button");
    }

    await goto(page, "/profile/history");
    const list = page.locator('[data-testid="history-list"]');
    await expect(list).toBeVisible({ timeout: 10_000 });
    const items = page.locator('[data-testid="history-item"]');
    expect(await items.count()).toBeGreaterThan(0);

    // cleanup
    await goto(page, `/media/${id}`);
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    await page.locator(unwatchSel).click().catch(() => {});
    await page.waitForTimeout(500);
  });

  test("clear history button shows confirmation modal", async ({ page }) => {
    await goto(page, "/profile/history");
    await page.waitForTimeout(2_000);

    const clearBtn = page.locator('[data-testid="clear-history-btn"]');
    if (!(await clearBtn.isVisible({ timeout: 3_000 }).catch(() => false))) {
      test.skip(true, "No clear button (empty history)");
    }

    await clearBtn.click();
    const modal = page.locator('[role="dialog"]');
    await expect(modal).toBeVisible({ timeout: 3_000 });

    // cancel should close modal
    await page.locator('[role="dialog"] button', { hasText: /cancel/i }).click();
    await expect(modal).not.toBeVisible({ timeout: 3_000 });
  });

  test("no JS errors on history page", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await goto(page, "/profile/history");
      await page.waitForTimeout(3_000);
    });
    expect(jsErrors).toHaveLength(0);
  });

  test("no 5xx on history page", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, "/profile/history");
      await page.waitForTimeout(3_000);
    });
    expect(serverErrors).toHaveLength(0);
  });
});
