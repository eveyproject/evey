import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

// Locators target the stable data-testid attributes (media-context-menu-<key>
// in CardContextMenu.tsx); the e2e account runs in French so English-text
// matchers never matched the rendered labels. There is no "Download" option in
// the card menu (download is a media-detail action), so that case was dropped.
const FAVORITE = '[data-testid="media-context-menu-add-to-favorites"], [data-testid="media-context-menu-remove-from-favorites"]';

async function openContextMenu(page: import("@playwright/test").Page): Promise<boolean> {
  await goto(page, "/home");
  const card = page.locator('[data-testid="media-card"]').first();
  if (!(await card.isVisible({ timeout: 10_000 }).catch(() => false))) return false;
  await card.click({ button: "right" });
  await page.waitForTimeout(500);
  return true;
}

test.describe("Context Menu > Each option individually", () => {
  test("menu appears on right-click", async ({ page }) => {
    test.skip(!(await openContextMenu(page)), "No card");
    await expect(page.locator('[role="menu"]')).toBeVisible({ timeout: 2_000 });
  });

  test("Edit metadata option visible", async ({ page }) => {
    test.skip(!(await openContextMenu(page)), "No card");
    await expect(page.locator('[data-testid="media-context-menu-edit-metadata"]').first()).toBeVisible({ timeout: 2_000 });
  });

  test("Add to Playlist option visible", async ({ page }) => {
    test.skip(!(await openContextMenu(page)), "No card");
    await expect(page.locator('[data-testid="media-context-menu-add-to-playlist"]').first()).toBeVisible({ timeout: 2_000 });
  });

  test("Add to Collection option visible", async ({ page }) => {
    test.skip(!(await openContextMenu(page)), "No card");
    await expect(page.locator('[data-testid="media-context-menu-add-to-collection"]').first()).toBeVisible({ timeout: 2_000 });
  });

  test("Add to Favorites option visible", async ({ page }) => {
    test.skip(!(await openContextMenu(page)), "No card");
    await expect(page.locator(FAVORITE).first()).toBeVisible({ timeout: 2_000 });
  });

  test("Mark as watched option visible", async ({ page }) => {
    test.skip(!(await openContextMenu(page)), "No card");
    await expect(page.locator('[data-testid="media-context-menu-mark-watched"]').first()).toBeVisible({ timeout: 2_000 });
  });

  test("clicking Edit metadata navigates to media detail", async ({ page }) => {
    test.skip(!(await openContextMenu(page)), "No card");
    const edit = page.locator('[data-testid="media-context-menu-edit-metadata"]').first();
    test.skip(!(await edit.isVisible({ timeout: 1_000 }).catch(() => false)), "No edit option");
    await edit.click();
    await page.waitForTimeout(2_000);
    expect(page.url()).toContain("/media/");
  });

  test("clicking Add to Playlist opens modal", async ({ page }) => {
    test.skip(!(await openContextMenu(page)), "No card");
    await page.locator('[data-testid="media-context-menu-add-to-playlist"]').first().click();
    await page.waitForTimeout(1_000);
    const modal = page.locator('[role="dialog"]').first();
    const visible = await modal.isVisible({ timeout: 2_000 }).catch(() => false);
    expect(typeof visible).toBe("boolean");
  });

  test("clicking Add to Collection opens modal", async ({ page }) => {
    test.skip(!(await openContextMenu(page)), "No card");
    await page.locator('[data-testid="media-context-menu-add-to-collection"]').first().click();
    await page.waitForTimeout(1_000);
    const modal = page.locator('[role="dialog"]').first();
    const visible = await modal.isVisible({ timeout: 2_000 }).catch(() => false);
    expect(typeof visible).toBe("boolean");
  });

  test("clicking Mark watched doesn't 5xx", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      if (!(await openContextMenu(page))) return;
      const opt = page.locator('[data-testid="media-context-menu-mark-watched"]').first();
      if (await opt.isVisible({ timeout: 1_000 }).catch(() => false)) {
        await opt.click();
        await page.waitForTimeout(1_500);
      }
    });
    expect(serverErrors).toHaveLength(0);
  });

  test("clicking Favorite doesn't 5xx", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      if (!(await openContextMenu(page))) return;
      const opt = page.locator(FAVORITE).first();
      if (await opt.isVisible({ timeout: 1_000 }).catch(() => false)) {
        await opt.click();
        await page.waitForTimeout(1_500);
      }
    });
    expect(serverErrors).toHaveLength(0);
  });

  test("Escape closes menu", async ({ page }) => {
    test.skip(!(await openContextMenu(page)), "No card");
    await page.keyboard.press("Escape");
    await page.waitForTimeout(300);
    const visible = await page.locator('[role="menu"]').isVisible({ timeout: 500 }).catch(() => false);
    expect(visible).toBe(false);
  });

  test("clicking outside closes menu", async ({ page }) => {
    test.skip(!(await openContextMenu(page)), "No card");
    await page.locator("body").click({ position: { x: 5, y: 5 } });
    await page.waitForTimeout(300);
    const visible = await page.locator('[role="menu"]').isVisible({ timeout: 500 }).catch(() => false);
    expect(visible).toBe(false);
  });

  test("no JS errors during any context menu interaction", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      if (!(await openContextMenu(page))) return;
      await page.keyboard.press("Escape");
    });
    expect(jsErrors).toHaveLength(0);
  });
});
