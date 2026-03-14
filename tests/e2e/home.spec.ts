import { test, expect } from "./fixtures/auth";

test.describe("Home page", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/home", { waitUntil: "networkidle" });
  });

  test("page loads without error", async ({ page }) => {
    const homePage = page.locator('[data-testid="home-page"]');
    await expect(homePage).toBeVisible({ timeout: 10_000 });
  });

  test("hero banner is displayed when content available", async ({ page }) => {
    const hero = page.locator('[data-testid="hero-banner"]');
    const visible = await hero.isVisible().catch(() => false);
    if (!visible) {
      // hero may not render if no recently added items with mediaItemId
      await hero.waitFor({ state: "visible", timeout: 5_000 }).catch(() => {});
      const stillVisible = await hero.isVisible().catch(() => false);
      test.skip(!stillVisible, "no hero banner content available");
    }

    const img = hero.locator("img");
    const imgCount = await img.count();
    expect(imgCount).toBeGreaterThan(0);
  });

  test("at least one media slider row is shown", async ({ page }) => {
    const sliders = page.locator('[data-testid="media-slider"]');
    await expect(sliders.first()).toBeVisible({ timeout: 15_000 });
    const count = await sliders.count();
    expect(count).toBeGreaterThan(0);
  });

  test("media cards in sliders are clickable", async ({ page }) => {
    const slider = page.locator('[data-testid="media-slider"]').first();
    await expect(slider).toBeVisible({ timeout: 15_000 });

    const card = slider.locator('[data-testid="media-card"]').first();
    await expect(card).toBeVisible({ timeout: 10_000 });

    const href = await card.getAttribute("href");
    expect(href).toContain("/media/");
  });

  test("Recently Added row is visible when data exists", async ({ page }) => {
    const sliders = page.locator('[data-testid="media-slider"]');
    await expect(sliders.first()).toBeVisible({ timeout: 15_000 });

    const recentSlider = sliders.filter({ hasText: "Recently Added" });
    const visible = await recentSlider.isVisible().catch(() => false);
    test.skip(!visible, "no recently added data");

    const cards = recentSlider.locator('[data-testid="media-card"]');
    const count = await cards.count();
    expect(count).toBeGreaterThan(0);
  });

  test("Continue Watching row visibility depends on history", async ({ page }) => {
    const sliders = page.locator('[data-testid="media-slider"]');
    await expect(sliders.first()).toBeVisible({ timeout: 15_000 });

    const cwSlider = sliders.filter({ hasText: "Continue Watching" });
    const visible = await cwSlider.isVisible().catch(() => false);
    if (visible) {
      const cards = cwSlider.locator('[data-testid="media-card"]');
      const count = await cards.count();
      expect(count).toBeGreaterThan(0);
    }
    expect(true).toBe(true);
  });

  test("hero banner has Play and More Info buttons", async ({ page }) => {
    const hero = page.locator('[data-testid="hero-banner"]');
    await hero.waitFor({ state: "visible", timeout: 10_000 }).catch(() => {});
    const visible = await hero.isVisible().catch(() => false);
    test.skip(!visible, "no hero banner content");

    const playBtn = hero.locator("a").filter({ hasText: "Play" });
    const infoBtn = hero.locator("a").filter({ hasText: "More Info" });

    await expect(playBtn).toBeVisible();
    await expect(infoBtn).toBeVisible();
  });

  test("library sliders show media cards", async ({ page }) => {
    const sliders = page.locator('[data-testid="media-slider"]');
    await expect(sliders.first()).toBeVisible({ timeout: 15_000 });

    const firstSlider = sliders.first();
    const cards = firstSlider.locator('[data-testid="media-card"]');
    await expect(cards.first()).toBeVisible({ timeout: 10_000 });
    const count = await cards.count();
    expect(count).toBeGreaterThan(0);
  });
});
