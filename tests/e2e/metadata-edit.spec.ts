import { test, expect } from "./fixtures/auth";

const MOVIE_ID = process.env.MEDIA_ID ?? "2fe73ab5-d888-432e-9a91-afdf2e98ae79";

test.describe("Metadata editing", () => {
  test("edit button is visible on media detail page", async ({ page }) => {
    await page.goto(`/media/${MOVIE_ID}`, { waitUntil: "networkidle" });
    const detail = page.locator('[data-testid="media-detail"]');
    await expect(detail).toBeVisible({ timeout: 15_000 });

    // Edit button is the pencil icon next to the title
    const editBtn = page.locator('button[title="Edit metadata"]');
    await expect(editBtn).toBeVisible({ timeout: 10_000 });
  });

  test("edit panel opens with ?edit=true and shows form fields", async ({ page }) => {
    await page.goto(`/media/${MOVIE_ID}?edit=true`, { waitUntil: "networkidle" });
    const detail = page.locator('[data-testid="media-detail"]');
    await expect(detail).toBeVisible({ timeout: 15_000 });

    // Form fields: Title, Year, Overview, Genres, Rating, IMDB ID, TMDB ID
    const titleInput = page.locator('input').filter({ has: page.locator('..', { hasText: 'Title' }) }).first();
    // Fallback: look for labeled inputs inside the edit form
    const form = page.locator("form");
    await expect(form).toBeVisible({ timeout: 10_000 });

    // Check key labels exist
    await expect(page.locator("text=Title").first()).toBeVisible({ timeout: 5_000 });
    await expect(page.locator("text=IMDB ID").first()).toBeVisible({ timeout: 5_000 });
    await expect(page.locator("text=TMDB ID").first()).toBeVisible({ timeout: 5_000 });

    // Save and Cancel buttons
    await expect(page.locator('button:has-text("Save")').first()).toBeVisible({ timeout: 5_000 });
    await expect(page.locator('button:has-text("Cancel")').first()).toBeVisible({ timeout: 5_000 });
    await expect(page.locator('button:has-text("Refresh metadata")').first()).toBeVisible({ timeout: 5_000 });
  });

  test("edit panel opens via pencil button click", async ({ page }) => {
    await page.goto(`/media/${MOVIE_ID}`, { waitUntil: "networkidle" });
    const detail = page.locator('[data-testid="media-detail"]');
    await expect(detail).toBeVisible({ timeout: 15_000 });

    const editBtn = page.locator('button[title="Edit metadata"]');
    await expect(editBtn).toBeVisible({ timeout: 10_000 });
    await editBtn.click();

    // Form should now appear
    const form = page.locator("form");
    await expect(form).toBeVisible({ timeout: 10_000 });
    await expect(page.locator('button:has-text("Save")').first()).toBeVisible({ timeout: 5_000 });
  });

  test("cancel button closes edit panel", async ({ page }) => {
    await page.goto(`/media/${MOVIE_ID}?edit=true`, { waitUntil: "networkidle" });
    const detail = page.locator('[data-testid="media-detail"]');
    await expect(detail).toBeVisible({ timeout: 15_000 });

    const form = page.locator("form");
    await expect(form).toBeVisible({ timeout: 10_000 });

    await page.locator('button:has-text("Cancel")').first().click();

    // Form should disappear
    await expect(form).not.toBeVisible({ timeout: 5_000 });
  });

  test("artwork picker section visible when artwork exists", async ({ page }) => {
    const artworkResponse = page.waitForResponse(
      (r) => r.url().includes(`/metadata/${MOVIE_ID}/artwork`) && r.status() === 200,
    );
    await page.goto(`/media/${MOVIE_ID}?edit=true`, { waitUntil: "networkidle" });
    const detail = page.locator('[data-testid="media-detail"]');
    await expect(detail).toBeVisible({ timeout: 15_000 });

    let hasMultipleArtwork = false;
    try {
      const res = await artworkResponse;
      const body = await res.json();
      const artwork: Array<{ type: string }> = body.artwork ?? [];
      const posters = artwork.filter((a) => a.type === "ARTWORK_TYPE_PRIMARY");
      const backdrops = artwork.filter((a) => a.type === "ARTWORK_TYPE_BACKDROP");
      hasMultipleArtwork = posters.length > 1 || backdrops.length > 1;
    } catch {
      // artwork endpoint may not fire if no edit panel or no artwork
    }

    test.skip(!hasMultipleArtwork, "Not enough artwork variants to show picker");

    // Poster or Backdrop label in artwork picker
    const artworkLabel = page.locator("text=/Poster|Backdrop/");
    await expect(artworkLabel.first()).toBeVisible({ timeout: 5_000 });
  });
});
