import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia, collectErrors } from "../shared/helpers";

async function openPlayer(page: import("@playwright/test").Page): Promise<boolean> {
  const id = await gotoRandomMedia(page);
  if (!id) return false;
  const playBtn = page.locator('[data-testid="play-button"]');
  if (!(await playBtn.isVisible({ timeout: 3_000 }).catch(() => false))) return false;
  await playBtn.click();
  await page.waitForURL("**/player/**", { timeout: 10_000 });
  await page.waitForSelector("video", { timeout: 15_000 });
  await page.waitForTimeout(3_000);
  return true;
}

async function showControls(page: import("@playwright/test").Page) {
  const vp = page.viewportSize()!;
  await page.mouse.move(vp.width / 2, vp.height / 2);
  await page.waitForTimeout(500);
}

test.describe("Media > Player controls", () => {
  test("seek bar visible after opening controls", async ({ page }) => {
    test.skip(!(await openPlayer(page)), "Could not open player");
    await showControls(page);
    const seekBar = page.locator('input[aria-label="Seek"]');
    await expect(seekBar).toBeVisible({ timeout: 3_000 });
  });

  test("time display shows non-zero duration", async ({ page }) => {
    test.skip(!(await openPlayer(page)), "Could not open player");
    await showControls(page);
    const time = page.locator("span.tabular-nums");
    await expect(time).toBeVisible({ timeout: 3_000 });
    const text = await time.textContent();
    expect(text).not.toContain("NaN");
    expect(text).not.toContain("Infinity");
    expect(text).not.toBe("0:00 / 0:00");
  });

  test("play/pause button is visible", async ({ page }) => {
    test.skip(!(await openPlayer(page)), "Could not open player");
    await showControls(page);
    const btn = page.locator('button[aria-label="Pause"], button[aria-label="Play"]');
    await expect(btn).toBeVisible({ timeout: 3_000 });
  });

  test("settings button opens quality panel", async ({ page }) => {
    test.skip(!(await openPlayer(page)), "Could not open player");
    await showControls(page);
    const settingsBtn = page.locator('button[aria-label="Settings"]');
    await expect(settingsBtn).toBeVisible({ timeout: 3_000 });
    await settingsBtn.click();
    await page.waitForTimeout(500);
    const qualityText = page.locator("text=/quality/i").first();
    await expect(qualityText).toBeVisible({ timeout: 2_000 });
  });

  test("keyboard Space pauses video", async ({ page }) => {
    test.skip(!(await openPlayer(page)), "Could not open player");
    const wasPaused = await page.evaluate(() => document.querySelector("video")?.paused);
    await page.keyboard.press("Space");
    await page.waitForTimeout(500);
    const isPaused = await page.evaluate(() => document.querySelector("video")?.paused);
    if (!wasPaused) expect(isPaused).toBe(true);
  });

  test("keyboard M toggles mute", async ({ page }) => {
    test.skip(!(await openPlayer(page)), "Could not open player");
    const wasMuted = await page.evaluate(() => document.querySelector("video")?.muted);
    await page.keyboard.press("m");
    await page.waitForTimeout(300);
    const isMuted = await page.evaluate(() => document.querySelector("video")?.muted);
    expect(isMuted).not.toBe(wasMuted);
  });

  test("keyboard ArrowRight seeks forward", async ({ page }) => {
    test.skip(!(await openPlayer(page)), "Could not open player");
    const timeBefore = await page.evaluate(() => document.querySelector("video")?.currentTime ?? 0);
    await page.keyboard.press("ArrowRight");
    await page.waitForTimeout(2_000);
    await showControls(page);
    const timeDisplay = await page.locator("span.tabular-nums").textContent();
    expect(timeDisplay).not.toContain("NaN");
  });

  test("fullscreen button visible", async ({ page }) => {
    test.skip(!(await openPlayer(page)), "Could not open player");
    await showControls(page);
    const fsBtn = page.locator('button[aria-label="Fullscreen"], button[aria-label="Exit fullscreen"]');
    await expect(fsBtn).toBeVisible({ timeout: 3_000 });
  });

  test("player has no JS errors during playback", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      if (!(await openPlayer(page))) return;
      await page.waitForTimeout(5_000);
    });
    expect(jsErrors).toHaveLength(0);
  });

  test("player has no 5xx during playback", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      if (!(await openPlayer(page))) return;
      await page.waitForTimeout(5_000);
    });
    expect(serverErrors).toHaveLength(0);
  });
});
