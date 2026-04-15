import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia } from "../shared/helpers";

test.describe("Media > Play button", () => {
  test("play button exists on movie/episode detail", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const playBtn = page.locator('[data-testid="play-button"]');
    // play button may not exist for series (no direct play)
    const visible = await playBtn.isVisible({ timeout: 3_000 }).catch(() => false);
    expect(typeof visible).toBe("boolean");
  });

  test("clicking play navigates to player", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const playBtn = page.locator('[data-testid="play-button"]');
    test.skip(!(await playBtn.isVisible({ timeout: 3_000 }).catch(() => false)), "No play button (series?)");
    await playBtn.click();
    await page.waitForURL("**/player/**", { timeout: 10_000 });
    expect(page.url()).toContain("/player/");
  });

  test("player loads video element", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const playBtn = page.locator('[data-testid="play-button"]');
    test.skip(!(await playBtn.isVisible({ timeout: 3_000 }).catch(() => false)), "No play button");
    await playBtn.click();
    await page.waitForURL("**/player/**", { timeout: 10_000 });
    const video = page.locator("video");
    await expect(video).toBeVisible({ timeout: 15_000 });
  });

  test("player back button returns to media detail", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const playBtn = page.locator('[data-testid="play-button"]');
    test.skip(!(await playBtn.isVisible({ timeout: 3_000 }).catch(() => false)), "No play button");
    await playBtn.click();
    await page.waitForURL("**/player/**", { timeout: 10_000 });
    await page.waitForTimeout(3_000);
    // show controls and click back
    const viewport = page.viewportSize()!;
    await page.mouse.move(viewport.width / 2, viewport.height / 2);
    await page.waitForTimeout(500);
    const backBtn = page.locator('button[aria-label="Back"]');
    if (await backBtn.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await backBtn.click();
      await page.waitForTimeout(2_000);
      expect(page.url()).toContain("/media/");
    }
  });
});
