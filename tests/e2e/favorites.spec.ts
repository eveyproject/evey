import { test, expect } from "./fixtures/auth";

const MOVIE_ID =
  process.env.MEDIA_ID ?? "577d3d4d-da77-4c8a-8e46-6bee6b64635c";

test.describe("Favorites", () => {
  test.describe.configure({ mode: "serial" });

  // Ensure movie starts unfavorited
  test("ensure movie is not favorited (cleanup)", async ({ page }) => {
    await page.goto(`/media/${MOVIE_ID}`, { waitUntil: "networkidle" });
    await page.waitForSelector('[data-testid="media-actions"]', {
      timeout: 15_000,
    });

    const actions = page.locator('[data-testid="media-actions"]');
    // Wait for the favorite button to render (either state)
    const anyFavBtn = actions.locator(
      '[data-testid="favorite-btn"], [data-testid="favorite-btn-active"]',
    );
    await expect(anyFavBtn).toBeVisible({ timeout: 10_000 });

    // If already favorited, toggle it off
    const removeBtn = actions.locator('[data-testid="favorite-btn-active"]');
    if (await removeBtn.isVisible().catch(() => false)) {
      await Promise.all([
        page.waitForResponse(
          (r) => r.url().includes("/api/v1/favorites/") && r.request().method() === "POST",
        ),
        removeBtn.click(),
      ]);
      await expect(
        actions.locator('[data-testid="favorite-btn"]'),
      ).toBeVisible({ timeout: 10_000 });
    }
  });

  test("add movie to favorites from media detail", async ({ page }) => {
    await page.goto(`/media/${MOVIE_ID}`, { waitUntil: "networkidle" });
    await page.waitForSelector('[data-testid="media-actions"]', {
      timeout: 15_000,
    });

    const actions = page.locator('[data-testid="media-actions"]');
    const favBtn = actions.locator('[data-testid="favorite-btn"]');
    await expect(favBtn).toBeVisible({ timeout: 10_000 });

    const [resp] = await Promise.all([
      page.waitForResponse(
        (r) => r.url().includes("/api/v1/favorites/") && r.request().method() === "POST",
        { timeout: 10_000 },
      ),
      favBtn.click(),
    ]);
    expect(resp.status()).toBeLessThan(400);

    await expect(
      actions.locator('[data-testid="favorite-btn-active"]'),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("favorited movie appears on /favorites page", async ({ page }) => {
    await page.goto("/favorites", { waitUntil: "networkidle" });

    // Should see the Favorites heading
    await expect(
      page.locator("h1", { hasText: "Favorites" }),
    ).toBeVisible({ timeout: 10_000 });

    // At least one media card should render (the movie we just added)
    const cards = page.locator("a[href*='/media/']");
    await expect(cards.first()).toBeVisible({ timeout: 15_000 });

    const count = await cards.count();
    expect(count).toBeGreaterThan(0);
  });

  test("favorites persist after page reload", async ({ page }) => {
    await page.goto("/favorites", { waitUntil: "networkidle" });

    await expect(
      page.locator("h1", { hasText: "Favorites" }),
    ).toBeVisible({ timeout: 10_000 });

    // Reload and verify the list is still populated
    await page.reload({ waitUntil: "networkidle" });

    const cards = page.locator("a[href*='/media/']");
    await expect(cards.first()).toBeVisible({ timeout: 15_000 });

    const count = await cards.count();
    expect(count).toBeGreaterThan(0);
  });

  test("remove movie from favorites", async ({ page }) => {
    await page.goto(`/media/${MOVIE_ID}`, { waitUntil: "networkidle" });
    await page.waitForSelector('[data-testid="media-detail"]', {
      timeout: 15_000,
    });

    const actions = page.locator('[data-testid="media-actions"]');
    const removeBtn = actions.locator('[data-testid="favorite-btn-active"]');
    await expect(removeBtn).toBeVisible({ timeout: 10_000 });

    const [resp] = await Promise.all([
      page.waitForResponse(
        (r) => r.url().includes("/api/v1/favorites/") && r.request().method() === "POST",
        { timeout: 10_000 },
      ),
      removeBtn.click(),
    ]);
    expect(resp.status()).toBeLessThan(400);

    await expect(
      actions.locator('[data-testid="favorite-btn"]'),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("removed movie disappears from /favorites page", async ({ page }) => {
    await page.goto("/favorites", { waitUntil: "networkidle" });

    await expect(
      page.locator("h1", { hasText: "Favorites" }),
    ).toBeVisible({ timeout: 10_000 });

    // Wait for content to load
    await page.locator("text=No favorites yet").or(page.locator('[data-testid="media-card"]').first()).waitFor({ timeout: 10_000 }).catch(() => {});

    // Either "No favorites yet" empty state or no card linking to our movie
    const emptyState = page.locator("text=No favorites yet");
    const movieLink = page.locator(`a[href*='/media/${MOVIE_ID}']`);

    const isEmpty = await emptyState.isVisible().catch(() => false);
    const hasMovie = await movieLink.isVisible().catch(() => false);

    // The specific movie should not be visible, or the page is empty
    expect(isEmpty || !hasMovie).toBe(true);
  });
});
