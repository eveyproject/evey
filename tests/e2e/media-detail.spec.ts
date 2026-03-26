import { test, expect } from "./fixtures/auth";

// Use env var or fallback to a known movie with metadata
const MOVIE_ID = process.env.MEDIA_ID ?? "577d3d4d-da77-4c8a-8e46-6bee6b64635c";
const SERIES_ID = process.env.SERIES_ID ?? "cda0bda7-a9d3-4e3b-b439-88ee774c9be9";

test.describe("Media Detail — movie", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto(`/media/${MOVIE_ID}`, { waitUntil: "networkidle" });
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
    await page.goto(`/media/${MOVIE_ID}`, { waitUntil: "networkidle" });

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

  // skip: backend bug — watch_progress FK constraint on profile_id
  test.skip("mark watched toggles state", async ({ page }) => {
    const detail = page.locator('[data-testid="media-detail"]');
    await expect(detail).toBeVisible({ timeout: 15_000 });

    const watchedBtn = page.locator('[data-testid="mark-watched-btn"]');
    const unwatchedBtn = page.locator('[data-testid="mark-unwatched-btn"]');
    await expect(watchedBtn).toBeVisible({ timeout: 10_000 });

    await watchedBtn.click();
    await expect(unwatchedBtn).toBeVisible({ timeout: 15_000 });

    await unwatchedBtn.click();
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

test.describe("Media Detail — series", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto(`/media/${SERIES_ID}`, { waitUntil: "networkidle" });
  });

  test("page loads without error", async ({ page }) => {
    const detail = page.locator('[data-testid="media-detail"]');
    await expect(detail).toBeVisible({ timeout: 15_000 });
  });

  test("seasons accordion is visible", async ({ page }) => {
    const seasons = page.locator('[data-testid="seasons-section"]');
    await expect(seasons).toBeVisible({ timeout: 15_000 });

    // Should contain at least one season row
    const seasonBtns = seasons.locator("button");
    const count = await seasonBtns.count();
    expect(count).toBeGreaterThan(0);
  });

  test("series has no download button (no filePath)", async ({ page }) => {
    const detail = page.locator('[data-testid="media-detail"]');
    await expect(detail).toBeVisible({ timeout: 15_000 });

    const dlBtn = page.locator('[data-testid="download-btn"]');
    await expect(dlBtn).not.toBeVisible({ timeout: 3_000 });
  });
});
