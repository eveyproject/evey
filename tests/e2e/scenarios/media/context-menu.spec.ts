import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

test.describe("Media > Context menu", () => {
  test("right-click on card opens context menu", async ({ page }) => {
    await goto(page, "/home");
    const card = page.locator('[data-testid="media-card"]').first();
    await card.waitFor({ state: "visible", timeout: 10_000 });
    await card.click({ button: "right" });
    await page.waitForTimeout(500);
    // menu should have at least one option
    const menu = page.locator('[role="menu"], [data-testid="context-menu"]');
    const menuVisible = await menu.isVisible({ timeout: 2_000 }).catch(() => false);
    // if no role=menu, check for floating options
    if (!menuVisible) {
      const options = page.locator("text=/download|playlist|collection|watched|favorite/i").first();
      await expect(options).toBeVisible({ timeout: 2_000 });
    }
  });

  test("context menu has 'Mark as watched' option", async ({ page }) => {
    await goto(page, "/home");
    const card = page.locator('[data-testid="media-card"]').first();
    await card.waitFor({ state: "visible", timeout: 10_000 });
    await card.click({ button: "right" });
    await page.waitForTimeout(500);
    const option = page.locator("text=/mark.*watched/i").first();
    await expect(option).toBeVisible({ timeout: 2_000 });
  });

  test("context menu has 'Download' option", async ({ page }) => {
    await goto(page, "/home");
    const card = page.locator('[data-testid="media-card"]').first();
    await card.waitFor({ state: "visible", timeout: 10_000 });
    await card.click({ button: "right" });
    await page.waitForTimeout(500);
    const option = page.locator("text=/download/i").first();
    await expect(option).toBeVisible({ timeout: 2_000 });
  });

  test("context menu has 'Add to Playlist' option", async ({ page }) => {
    await goto(page, "/home");
    const card = page.locator('[data-testid="media-card"]').first();
    await card.waitFor({ state: "visible", timeout: 10_000 });
    await card.click({ button: "right" });
    await page.waitForTimeout(500);
    const option = page.locator("text=/playlist/i").first();
    await expect(option).toBeVisible({ timeout: 2_000 });
  });

  test("context menu has 'Add to Collection' option", async ({ page }) => {
    await goto(page, "/home");
    const card = page.locator('[data-testid="media-card"]').first();
    await card.waitFor({ state: "visible", timeout: 10_000 });
    await card.click({ button: "right" });
    await page.waitForTimeout(500);
    const option = page.locator("text=/collection/i").first();
    await expect(option).toBeVisible({ timeout: 2_000 });
  });

  test("clicking outside closes context menu", async ({ page }) => {
    await goto(page, "/home");
    const card = page.locator('[data-testid="media-card"]').first();
    await card.waitFor({ state: "visible", timeout: 10_000 });
    await card.click({ button: "right" });
    await page.waitForTimeout(500);
    // click away
    await page.locator("body").click({ position: { x: 10, y: 10 } });
    await page.waitForTimeout(500);
    const option = page.locator("text=/download/i").first();
    const visible = await option.isVisible({ timeout: 1_000 }).catch(() => false);
    expect(visible).toBe(false);
  });

  test("context menu mark watched has no errors", async ({ page }) => {
    const { jsErrors, serverErrors } = await collectErrors(page, async () => {
      await goto(page, "/home");
      const card = page.locator('[data-testid="media-card"]').first();
      await card.waitFor({ state: "visible", timeout: 10_000 });
      await card.click({ button: "right" });
      await page.waitForTimeout(500);
      const watchOption = page.locator("text=/mark.*watched/i").first();
      if (await watchOption.isVisible({ timeout: 2_000 }).catch(() => false)) {
        await watchOption.click();
        await page.waitForTimeout(1_500);
      }
    });
    expect(jsErrors).toHaveLength(0);
    expect(serverErrors).toHaveLength(0);
  });
});
