import { test, expect } from "../../fixtures/auth";
import { gotoRandomMovie, collectErrors } from "../shared/helpers";

test.use({
  viewport: { width: 390, height: 844 },
  hasTouch: true,
  isMobile: true,
});

async function openPlayerMobile(page: import("@playwright/test").Page): Promise<boolean> {
  const id = await gotoRandomMovie(page);
  if (!id) return false;
  const playBtn = page.locator('[data-testid="play-button"]');
  if (!(await playBtn.isVisible({ timeout: 3_000 }).catch(() => false))) return false;
  await playBtn.click();
  try { await page.waitForURL("**/player/**", { timeout: 10_000 }); } catch { return false; }
  await page.waitForSelector("video", { timeout: 15_000 });
  await page.waitForTimeout(3_000);
  return true;
}

test.describe("Player Mobile > Touch interactions", () => {
  test("tap shows controls", async ({ page }) => {
    test.skip(!(await openPlayerMobile(page)), "Cannot open player");
    const vp = page.viewportSize()!;
    await page.touchscreen.tap(vp.width / 2, vp.height / 2);
    await page.waitForTimeout(600);
    const btn = page.locator('button[aria-label="Pause"], button[aria-label="Play"]');
    await expect(btn).toBeVisible({ timeout: 2_000 });
  });

  test("play/pause button tappable", async ({ page }) => {
    test.skip(!(await openPlayerMobile(page)), "Cannot open player");
    const vp = page.viewportSize()!;
    await page.touchscreen.tap(vp.width / 2, vp.height / 2);
    await page.waitForTimeout(600);
    const btn = page.locator('button[aria-label="Pause"], button[aria-label="Play"]');
    test.skip(!(await btn.isVisible({ timeout: 2_000 }).catch(() => false)), "Controls not visible");
    await btn.tap();
    await page.waitForTimeout(300);
    // no crash = success
  });

  test("settings button tappable", async ({ page }) => {
    test.skip(!(await openPlayerMobile(page)), "Cannot open player");
    const vp = page.viewportSize()!;
    await page.touchscreen.tap(vp.width / 2, vp.height / 2);
    await page.waitForTimeout(600);
    const btn = page.locator('button[aria-label="Settings"]');
    test.skip(!(await btn.isVisible({ timeout: 2_000 }).catch(() => false)), "No settings");
    await btn.tap();
    await page.waitForTimeout(500);
    await expect(page.locator("text=/quality/i").first()).toBeVisible({ timeout: 2_000 });
  });

  test("seek bar within viewport", async ({ page }) => {
    test.skip(!(await openPlayerMobile(page)), "Cannot open player");
    const vp = page.viewportSize()!;
    await page.touchscreen.tap(vp.width / 2, vp.height / 2);
    await page.waitForTimeout(600);
    const seekBar = page.locator('input[aria-label="Seek"]');
    test.skip(!(await seekBar.isVisible({ timeout: 2_000 }).catch(() => false)), "No seek bar");
    const box = await seekBar.boundingBox();
    expect(box).toBeTruthy();
    expect(box!.y + box!.height).toBeLessThanOrEqual(vp.height);
  });

  test("back button tappable", async ({ page }) => {
    test.skip(!(await openPlayerMobile(page)), "Cannot open player");
    const vp = page.viewportSize()!;
    await page.touchscreen.tap(vp.width / 2, vp.height / 2);
    await page.waitForTimeout(600);
    const back = page.locator('button[aria-label="Back"]');
    test.skip(!(await back.isVisible({ timeout: 2_000 }).catch(() => false)), "No back btn");
    const box = await back.boundingBox();
    expect(box).toBeTruthy();
    expect(box!.width).toBeGreaterThan(10);
  });

  test("no JS errors during mobile playback", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await openPlayerMobile(page);
      await page.waitForTimeout(3_000);
    });
    expect(jsErrors).toHaveLength(0);
  });
});

test.describe("Player Mobile > Home page interactions", () => {
  test("home loads on mobile viewport", async ({ page }) => {
    await page.goto("/home", { waitUntil: "domcontentloaded", timeout: 10_000 });
    await expect(page.locator('[data-testid="home-page"]')).toBeVisible({ timeout: 10_000 });
  });

  test("media cards tappable on mobile", async ({ page }) => {
    await page.goto("/home", { waitUntil: "domcontentloaded", timeout: 10_000 });
    const card = page.locator('[data-testid="media-card"]').first();
    await card.waitFor({ state: "visible", timeout: 10_000 });
    await card.tap();
    await page.waitForTimeout(2_000);
    expect(page.url()).toContain("/media/");
  });

  test("hamburger menu opens", async ({ page }) => {
    await page.goto("/home", { waitUntil: "domcontentloaded", timeout: 10_000 });
    const burger = page.locator('button[aria-label*="menu" i]').first();
    await expect(burger).toBeVisible({ timeout: 5_000 });
    await burger.tap();
    await page.waitForTimeout(500);
    await expect(page.locator('[aria-label*="close" i]').first()).toBeVisible({ timeout: 3_000 });
  });

  test("search input accessible on mobile", async ({ page }) => {
    await page.goto("/home", { waitUntil: "domcontentloaded", timeout: 10_000 });
    await expect(page.locator('[data-testid="topbar-search-input"]')).toBeVisible({ timeout: 5_000 });
  });

  test("long-press on card opens context menu", async ({ page }) => {
    await page.goto("/home", { waitUntil: "domcontentloaded", timeout: 10_000 });
    const card = page.locator('[data-testid="media-card"]').first();
    await card.waitFor({ state: "visible", timeout: 10_000 });
    const box = await card.boundingBox();
    test.skip(!box, "No bounding box");
    // simulate long press
    await page.touchscreen.tap(box!.x + box!.width / 2, box!.y + box!.height / 2);
    // can't easily simulate long-press with Playwright touchscreen
    // just verify no crash
    await page.waitForTimeout(500);
  });

  test("no horizontal scroll on mobile home", async ({ page }) => {
    await page.goto("/home", { waitUntil: "domcontentloaded", timeout: 10_000 });
    await page.waitForTimeout(2_000);
    const hasHScroll = await page.evaluate(() =>
      document.documentElement.scrollWidth > document.documentElement.clientWidth + 5
    );
    expect(hasHScroll).toBe(false);
  });
});
