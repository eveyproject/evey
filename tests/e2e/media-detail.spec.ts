import { test, expect } from "./fixtures/auth";
import { gotoRandomMovie, gotoRandomSeries } from "./scenarios/shared/helpers";

// MEDIA_ID / SERIES_ID can pin a specific item (CI determinism); otherwise we
// pick a random one whose detail page loads - the previous hardcoded fallback
// IDs are 404 against any DB that wasn't the one they were captured from.
const MOVIE_ID = process.env.MEDIA_ID ?? "";
const SERIES_ID = process.env.SERIES_ID ?? "";

test.describe("Media Detail - movie", () => {
  test.beforeEach(async ({ page }) => {
    if (MOVIE_ID) {
      await page.goto(`/media/${MOVIE_ID}`, { waitUntil: "domcontentloaded" });
    } else {
      const id = await gotoRandomMovie(page);
      test.skip(!id, "No playable movie found");
    }
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 15_000 });
  });

  test("page loads without error", async ({ page }) => {
    const detail = page.locator('[data-testid="media-detail"]');
    await expect(detail).toBeVisible({ timeout: 15_000 });
  });

  test("title is displayed", async ({ page }) => {
    const title = page.locator('[data-testid="media-title"]');
    await expect(title).toBeVisible({ timeout: 10_000 });
    const text = await title.textContent();
    expect(text!.trim().length).toBeGreaterThan(0);
  });

  test("backdrop image is present", async ({ page }) => {
    const backdrop = page.locator('[data-testid="media-backdrop"]');
    await expect(backdrop).toBeVisible({ timeout: 10_000 });
    const src = await backdrop.getAttribute("src");
    expect(src).toBeTruthy();
  });

  test("poster image is present (desktop)", async ({ page }) => {
    // Poster is hidden on mobile (md:block), ensure viewport is wide enough
    await page.setViewportSize({ width: 1280, height: 800 });
    if (MOVIE_ID) {
      await page.goto(`/media/${MOVIE_ID}`, { waitUntil: "domcontentloaded" });
    } else {
      const id = await gotoRandomMovie(page);
      test.skip(!id, "No playable movie found");
    }
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 15_000 });

    const poster = page.locator('[data-testid="media-poster"]');
    await expect(poster).toBeVisible({ timeout: 10_000 });
    const src = await poster.getAttribute("src");
    expect(src).toBeTruthy();
  });

  test("genre chips are displayed", async ({ page }) => {
    const genres = page.locator('[data-testid="media-genres"]');
    await expect(genres).toBeVisible({ timeout: 10_000 });

    const chips = genres.locator("span");
    const count = await chips.count();
    expect(count).toBeGreaterThan(0);
  });

  test("play button is visible", async ({ page }) => {
    const playBtn = page.locator('[data-testid="play-button"]');
    await expect(playBtn).toBeVisible({ timeout: 10_000 });
  });

  test("overview is displayed", async ({ page }) => {
    const overview = page.locator('[data-testid="media-overview"]');
    await expect(overview).toBeVisible({ timeout: 10_000 });
    const text = await overview.textContent();
    expect(text!.trim().length).toBeGreaterThan(10);
  });

  test("mark watched toggles state", async ({ page }) => {
    const detail = page.locator('[data-testid="media-detail"]');
    await expect(detail).toBeVisible({ timeout: 15_000 });

    const watchedBtn = page.locator('[data-testid="mark-watched-btn"]');
    const unwatchedBtn = page.locator('[data-testid="mark-unwatched-btn"]');
    await expect(watchedBtn).toBeVisible({ timeout: 10_000 });

    await Promise.all([
      page.waitForResponse((r) => r.url().includes("/api/v1/playback/") && r.request().method() === "POST", { timeout: 10_000 }),
      watchedBtn.click(),
    ]);
    await expect(unwatchedBtn).toBeVisible({ timeout: 15_000 });

    await Promise.all([
      page.waitForResponse((r) => r.url().includes("/api/v1/playback/") && r.request().method() === "POST", { timeout: 10_000 }),
      unwatchedBtn.click(),
    ]);
    await expect(watchedBtn).toBeVisible({ timeout: 15_000 });
  });

  test("cast section has at least 1 person", async ({ page }) => {
    const cast = page.locator('[data-testid="cast-section"]');
    await expect(cast).toBeVisible({ timeout: 10_000 });

    // Each cast card is a link with class containing "w-28"
    const cards = cast.locator("a");
    const count = await cards.count();
    expect(count).toBeGreaterThan(0);
  });

  test("download button is visible", async ({ page }) => {
    const dlBtn = page.locator('[data-testid="download-btn"]');
    await expect(dlBtn).toBeVisible({ timeout: 10_000 });
    const href = await dlBtn.getAttribute("href");
    expect(href).toContain("/download");
  });
});

test.describe("Media Detail - series", () => {
  test.beforeEach(async ({ page }) => {
    if (SERIES_ID) {
      await page.goto(`/media/${SERIES_ID}`, { waitUntil: "domcontentloaded" });
    } else {
      const id = await gotoRandomSeries(page);
      test.skip(!id, "No series found");
    }
  });

  test("page loads without error", async ({ page }) => {
    const detail = page.locator('[data-testid="media-detail"]');
    await expect(detail).toBeVisible({ timeout: 15_000 });
  });

  test("the seasons section lists at least one season", async ({ page }) => {
    const seasons = page.locator('[data-testid="seasons-section"]');
    await expect(seasons).toBeVisible({ timeout: 15_000 });

    // Count season TILES, not buttons. The section stopped being an accordion
    // of buttons a while back - it is a grid of poster tiles linking to each
    // season - and the only <button> left in it was the gapless-playback
    // toggle, since removed. Counting buttons was therefore asserting on a
    // control that had nothing to do with seasons, and went to zero the day it
    // was dropped.
    const tiles = seasons.locator('a[href^="/media/"]');
    await expect.poll(() => tiles.count(), { timeout: 15_000 }).toBeGreaterThan(0);
  });

  test("series has a download button (full-series ZIP)", async ({ page }) => {
    const detail = page.locator('[data-testid="media-detail"]');
    await expect(detail).toBeVisible({ timeout: 15_000 });

    const dlBtn = page.locator('[data-testid="download-btn"]').first();
    await expect(dlBtn).toBeVisible({ timeout: 10_000 });
    const href = await dlBtn.getAttribute("href");
    expect(href).toContain("/download");
  });
});
