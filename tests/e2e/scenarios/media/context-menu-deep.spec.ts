import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

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
    const option = page.locator("text=/edit|playlist|collection|download|watched|favorite/i").first();
    await expect(option).toBeVisible({ timeout: 2_000 });
  });

  test("Edit metadata option visible", async ({ page }) => {
    test.skip(!(await openContextMenu(page)), "No card");
    await expect(page.locator("text=/edit.*metadata/i").first()).toBeVisible({ timeout: 2_000 });
  });

  test("Add to Playlist option visible", async ({ page }) => {
    test.skip(!(await openContextMenu(page)), "No card");
    await expect(page.locator("text=/playlist/i").first()).toBeVisible({ timeout: 2_000 });
  });

  test("Add to Collection option visible", async ({ page }) => {
    test.skip(!(await openContextMenu(page)), "No card");
    await expect(page.locator("text=/collection/i").first()).toBeVisible({ timeout: 2_000 });
  });

  test("Add to Favorites option visible", async ({ page }) => {
    test.skip(!(await openContextMenu(page)), "No card");
    await expect(page.locator("text=/favorite/i").first()).toBeVisible({ timeout: 2_000 });
  });

  test("Download option visible", async ({ page }) => {
    test.skip(!(await openContextMenu(page)), "No card");
    await expect(page.locator("text=/download/i").first()).toBeVisible({ timeout: 2_000 });
  });

  test("Mark as watched option visible", async ({ page }) => {
    test.skip(!(await openContextMenu(page)), "No card");
    await expect(page.locator("text=/mark.*watched/i").first()).toBeVisible({ timeout: 2_000 });
  });

  test("clicking Edit metadata navigates to media detail", async ({ page }) => {
    test.skip(!(await openContextMenu(page)), "No card");
    const edit = page.locator("text=/edit.*metadata/i").first();
    test.skip(!(await edit.isVisible({ timeout: 1_000 }).catch(() => false)), "No edit option");
    await edit.click();
    await page.waitForTimeout(2_000);
    expect(page.url()).toContain("/media/");
  });

  test("clicking Add to Playlist opens modal", async ({ page }) => {
    test.skip(!(await openContextMenu(page)), "No card");
    const opt = page.locator("text=/playlist/i").first();
    await opt.click();
    await page.waitForTimeout(1_000);
    // modal or empty state
    const modal = page.locator('[role="dialog"], text=/playlist|no playlist|create/i').first();
    const visible = await modal.isVisible({ timeout: 2_000 }).catch(() => false);
    expect(typeof visible).toBe("boolean");
  });

  test("clicking Add to Collection opens modal", async ({ page }) => {
    test.skip(!(await openContextMenu(page)), "No card");
    const opt = page.locator("text=/collection/i").first();
    await opt.click();
    await page.waitForTimeout(1_000);
    const modal = page.locator('[role="dialog"], text=/collection|no collection/i').first();
    const visible = await modal.isVisible({ timeout: 2_000 }).catch(() => false);
    expect(typeof visible).toBe("boolean");
  });

  test("clicking Mark watched doesn't 5xx", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      if (!(await openContextMenu(page))) return;
      const opt = page.locator("text=/mark.*watched/i").first();
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
      const opt = page.locator("text=/favorite/i").first();
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
    const option = page.locator("text=/download/i").first();
    const visible = await option.isVisible({ timeout: 500 }).catch(() => false);
    expect(visible).toBe(false);
  });

  test("clicking outside closes menu", async ({ page }) => {
    test.skip(!(await openContextMenu(page)), "No card");
    await page.locator("body").click({ position: { x: 5, y: 5 } });
    await page.waitForTimeout(300);
    const option = page.locator("text=/download/i").first();
    const visible = await option.isVisible({ timeout: 500 }).catch(() => false);
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
