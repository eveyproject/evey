import { test, expect } from "../../fixtures/auth";
import { gotoRandomMovie, collectErrors, showPlayerControls } from "../shared/helpers";
import type { Page } from "@playwright/test";

async function openPlayer(page: Page): Promise<string | null> {
  const id = await gotoRandomMovie(page);
  if (!id) return null;
  const playBtn = page.locator('[data-testid="play-button"]');
  if (!(await playBtn.isVisible({ timeout: 5_000 }).catch(() => false))) return null;
  await playBtn.click();
  await page.waitForURL("**/player/**", { timeout: 10_000 });
  await page.waitForSelector("video", { timeout: 15_000 });
  await page.waitForTimeout(2_000);
  return id;
}

test.describe("Media > Player (deep)", () => {
  test("video element has src", async ({ page }) => {
    const id = await openPlayer(page);
    test.skip(!id, "No playable movie");

    const src = await page.evaluate(() => {
      const v = document.querySelector("video");
      return v?.src || v?.querySelector("source")?.src || v?.currentSrc || "";
    });
    expect(src.length).toBeGreaterThan(0);
  });

  test("seek bar max > 0", async ({ page }) => {
    const id = await openPlayer(page);
    test.skip(!id, "No playable movie");

    await showPlayerControls(page);
    const seekBar = page.locator('input[aria-label="Seek"]');
    const visible = await seekBar.isVisible({ timeout: 3_000 }).catch(() => false);
    test.skip(!visible, "No seek bar");

    const max = await seekBar.getAttribute("max");
    expect(Number(max)).toBeGreaterThan(0);
  });

  test("time display valid (not NaN, not 0:00/0:00)", async ({ page }) => {
    const id = await openPlayer(page);
    test.skip(!id, "No playable movie");

    await showPlayerControls(page);
    const time = page.locator("span.tabular-nums");
    await expect(time).toBeVisible({ timeout: 5_000 });
    const text = await time.textContent();
    expect(text).not.toContain("NaN");
    expect(text).not.toContain("Infinity");
    expect(text).not.toBe("0:00 / 0:00");
  });

  test("Space pauses → verify video.paused === true", async ({ page }) => {
    const id = await openPlayer(page);
    test.skip(!id, "No playable movie");

    // ensure playing
    const wasPaused = await page.evaluate(() => document.querySelector("video")?.paused);
    if (wasPaused) {
      await page.keyboard.press("Space");
      await page.waitForTimeout(500);
    }
    // now video should be playing
    const playing = await page.evaluate(() => !document.querySelector("video")?.paused);
    expect(playing).toBe(true);

    // press space to pause
    await page.keyboard.press("Space");
    await page.waitForTimeout(500);
    const isPaused = await page.evaluate(() => document.querySelector("video")?.paused);
    expect(isPaused).toBe(true);
  });

  test("Space resumes → verify video.paused === false", async ({ page }) => {
    const id = await openPlayer(page);
    test.skip(!id, "No playable movie");

    // ensure paused
    const wasPaused = await page.evaluate(() => document.querySelector("video")?.paused);
    if (!wasPaused) {
      await page.keyboard.press("Space");
      await page.waitForTimeout(500);
    }
    expect(await page.evaluate(() => document.querySelector("video")?.paused)).toBe(true);

    // press space to resume
    await page.keyboard.press("Space");
    await page.waitForTimeout(500);
    const isPlaying = await page.evaluate(() => !document.querySelector("video")?.paused);
    expect(isPlaying).toBe(true);
  });

  test("M toggles mute → verify video.muted changed", async ({ page }) => {
    const id = await openPlayer(page);
    test.skip(!id, "No playable movie");

    const wasMuted = await page.evaluate(() => document.querySelector("video")?.muted);
    await page.keyboard.press("m");
    await page.waitForTimeout(300);
    const isMuted = await page.evaluate(() => document.querySelector("video")?.muted);
    expect(isMuted).not.toBe(wasMuted);

    // toggle back
    await page.keyboard.press("m");
    await page.waitForTimeout(300);
  });

  test("ArrowRight → time advances", async ({ page }) => {
    const id = await openPlayer(page);
    test.skip(!id, "No playable movie");

    const timeBefore = await page.evaluate(() => document.querySelector("video")?.currentTime ?? 0);
    await page.keyboard.press("ArrowRight");
    await page.waitForTimeout(1_000);
    const timeAfter = await page.evaluate(() => document.querySelector("video")?.currentTime ?? 0);
    expect(timeAfter).toBeGreaterThan(timeBefore);
  });

  test("settings → quality section visible", async ({ page }) => {
    const id = await openPlayer(page);
    test.skip(!id, "No playable movie");

    await showPlayerControls(page);
    const settingsBtn = page.locator('button[aria-label="Settings"]');
    await expect(settingsBtn).toBeVisible({ timeout: 3_000 });
    await settingsBtn.click();
    await page.waitForTimeout(500);

    const qualityText = page.locator("text=/quality/i").first();
    await expect(qualityText).toBeVisible({ timeout: 3_000 });
  });

  test("settings → change speed to 1.5x → verify playbackRate", async ({ page }) => {
    const id = await openPlayer(page);
    test.skip(!id, "No playable movie");

    await showPlayerControls(page);
    const settingsBtn = page.locator('button[aria-label="Settings"]');
    await expect(settingsBtn).toBeVisible({ timeout: 3_000 });
    await settingsBtn.click();
    await page.waitForTimeout(500);

    // look for speed option
    const speedOption = page.locator("text=/speed/i").first();
    const hasSpeed = await speedOption.isVisible({ timeout: 2_000 }).catch(() => false);
    if (hasSpeed) {
      await speedOption.click();
      await page.waitForTimeout(500);
    }

    const opt15 = page.locator("text=/1\\.5/").first();
    const has15 = await opt15.isVisible({ timeout: 2_000 }).catch(() => false);
    test.skip(!has15, "No 1.5x speed option");

    await opt15.click();
    await page.waitForTimeout(500);
    const rate = await page.evaluate(() => document.querySelector("video")?.playbackRate);
    expect(rate).toBe(1.5);

    // reset speed
    await showPlayerControls(page);
    await settingsBtn.click();
    await page.waitForTimeout(500);
    if (hasSpeed) {
      await page.locator("text=/speed/i").first().click();
      await page.waitForTimeout(500);
    }
    const normal = page.locator("text=/1x$|^1$/").first();
    if (await normal.isVisible({ timeout: 1_000 }).catch(() => false)) {
      await normal.click();
    }
  });

  test("back button → returns to /media/{id}", async ({ page }) => {
    const id = await openPlayer(page);
    test.skip(!id, "No playable movie");

    await showPlayerControls(page);
    const backBtn = page.locator('button[aria-label="Back"]');
    const hasBack = await backBtn.isVisible({ timeout: 3_000 }).catch(() => false);
    test.skip(!hasBack, "No back button");

    await backBtn.click();
    await page.waitForTimeout(2_000);
    expect(page.url()).toContain("/media/");
  });

  test("no 5xx during entire player session", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      const id = await openPlayer(page);
      if (!id) return;
      await page.waitForTimeout(2_000);
      await page.keyboard.press("ArrowRight");
      await page.waitForTimeout(1_000);
    });
    expect(serverErrors, `5xx: ${serverErrors.join(", ")}`).toHaveLength(0);
  });

  test("no JS errors during entire player session", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      const id = await openPlayer(page);
      if (!id) return;
      await page.waitForTimeout(2_000);
      await page.keyboard.press("Space");
      await page.waitForTimeout(500);
      await page.keyboard.press("Space");
      await page.waitForTimeout(500);
    });
    expect(jsErrors).toHaveLength(0);
  });
});
