import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

// Locators target the menu's stable data-testid attributes
// (media-context-menu-<key> in CardContextMenu.tsx) rather than visible label
// text: the e2e account runs in French, so English-text matchers like
// /mark.*watched/i never matched the rendered "Marquer comme vu".
test.describe("Media > Context menu", () => {
  async function openCardMenu(page: import("@playwright/test").Page) {
    await goto(page, "/home");
    const card = page.locator('[data-testid="media-card"]').first();
    await card.waitFor({ state: "visible", timeout: 10_000 });
    await card.click({ button: "right" });
    await page.waitForTimeout(500);
  }

  test("right-click on card opens context menu", async ({ page }) => {
    await openCardMenu(page);
    await expect(page.locator('[role="menu"]')).toBeVisible({ timeout: 2_000 });
  });

  test("context menu has 'Mark as watched' option", async ({ page }) => {
    await openCardMenu(page);
    await expect(
      page.locator('[data-testid="media-context-menu-mark-watched"]'),
    ).toBeVisible({ timeout: 2_000 });
  });

  test("context menu has 'Add to Playlist' option", async ({ page }) => {
    await openCardMenu(page);
    await expect(
      page.locator('[data-testid="media-context-menu-add-to-playlist"]'),
    ).toBeVisible({ timeout: 2_000 });
  });

  test("context menu has 'Add to Collection' option", async ({ page }) => {
    await openCardMenu(page);
    await expect(
      page.locator('[data-testid="media-context-menu-add-to-collection"]'),
    ).toBeVisible({ timeout: 2_000 });
  });

  test("clicking outside closes context menu", async ({ page }) => {
    await openCardMenu(page);
    await page.locator("body").click({ position: { x: 10, y: 10 } });
    await page.waitForTimeout(500);
    const option = page.locator('[data-testid="media-context-menu-mark-watched"]');
    const visible = await option.isVisible({ timeout: 1_000 }).catch(() => false);
    expect(visible).toBe(false);
  });

  test("context menu mark watched has no errors", async ({ page }) => {
    const { jsErrors, serverErrors } = await collectErrors(page, async () => {
      await openCardMenu(page);
      const watchOption = page.locator('[data-testid="media-context-menu-mark-watched"]');
      if (await watchOption.isVisible({ timeout: 2_000 }).catch(() => false)) {
        await watchOption.click();
        await page.waitForTimeout(1_500);
      }
    });
    expect(jsErrors).toHaveLength(0);
    expect(serverErrors).toHaveLength(0);
  });
});
