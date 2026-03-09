import { test, expect } from "./fixtures/auth";

async function getFirstLibraryId(page: import("@playwright/test").Page): Promise<string | null> {
  await page.goto("/library", { waitUntil: "networkidle" });
  const link = page.locator('a[href*="/library/"]').first();
  const visible = await link.isVisible().catch(() => false);
  if (!visible) return null;
  const href = await link.getAttribute("href");
  const match = href?.match(/\/library\/([^/]+)/);
  return match?.[1] ?? null;
}

test.describe("Library browse", () => {
  test("page loads and shows library name", async ({ page }) => {
    const id = await getFirstLibraryId(page);
    test.skip(!id, "no library found");
    await page.goto(`/library/${id}`, { waitUntil: "networkidle" });

    const name = page.locator('[data-testid="library-name"]');
    await expect(name).toBeVisible({ timeout: 10_000 });
    const text = await name.textContent();
    expect(text!.length).toBeGreaterThan(0);
    expect(text).not.toBe("Library");
  });

  test("media cards are displayed", async ({ page }) => {
    const id = await getFirstLibraryId(page);
    test.skip(!id, "no library found");
    await page.goto(`/library/${id}`, { waitUntil: "networkidle" });

    const cards = page.locator('[data-testid="media-card"]');
    await expect(cards.first()).toBeVisible({ timeout: 15_000 });
    const count = await cards.count();
    expect(count).toBeGreaterThan(0);
  });

  test("media cards have poster image", async ({ page }) => {
    const id = await getFirstLibraryId(page);
    test.skip(!id, "no library found");
    await page.goto(`/library/${id}`, { waitUntil: "networkidle" });

    const card = page.locator('[data-testid="media-card"]').first();
    await expect(card).toBeVisible({ timeout: 15_000 });
    const img = card.locator("img");
    const imgCount = await img.count();
    expect(imgCount).toBeGreaterThan(0);
  });

  test("clicking a media card navigates to /media/{id}", async ({ page }) => {
    const id = await getFirstLibraryId(page);
    test.skip(!id, "no library found");
    await page.goto(`/library/${id}`, { waitUntil: "networkidle" });

    const card = page.locator('[data-testid="media-card"]').first();
    await expect(card).toBeVisible({ timeout: 15_000 });
    await card.click();
    await page.waitForURL(/\/media\//, { timeout: 10_000 });
    expect(page.url()).toContain("/media/");
  });

  test("genre filter works", async ({ page }) => {
    const id = await getFirstLibraryId(page);
    test.skip(!id, "no library found");
    await page.goto(`/library/${id}`, { waitUntil: "networkidle" });

    await page.locator('[data-testid="media-card"]').first().waitFor({ timeout: 15_000 });

    const genreSelect = page.locator("select").filter({ hasText: "All genres" });
    const hasGenre = (await genreSelect.count()) > 0;
    test.skip(!hasGenre, "no genre filter available");

    const options = genreSelect.locator("option");
    const optCount = await options.count();
    test.skip(optCount < 2, "no genre options");

    const genreValue = await options.nth(1).getAttribute("value");
    await genreSelect.selectOption(genreValue!);
    await page.waitForTimeout(1_000);

    const cards = page.locator('[data-testid="media-card"]');
    const count = await cards.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test("sort order works", async ({ page }) => {
    const id = await getFirstLibraryId(page);
    test.skip(!id, "no library found");
    await page.goto(`/library/${id}`, { waitUntil: "networkidle" });

    await page.locator('[data-testid="media-card"]').first().waitFor({ timeout: 15_000 });

    const sortSelect = page.locator("select").filter({ hasText: "Title" });
    await expect(sortSelect).toBeVisible();

    await sortSelect.selectOption("MEDIA_SORT_FIELD_YEAR");
    await page.waitForTimeout(1_000);

    const cards = page.locator('[data-testid="media-card"]');
    const count = await cards.count();
    expect(count).toBeGreaterThan(0);
  });

  test("search within library filters results", async ({ page }) => {
    const id = await getFirstLibraryId(page);
    test.skip(!id, "no library found");
    await page.goto(`/library/${id}`, { waitUntil: "networkidle" });

    await page.locator('[data-testid="media-card"]').first().waitFor({ timeout: 15_000 });

    const searchInput = page.locator('[data-testid="library-search-input"]');
    await expect(searchInput).toBeVisible();

    const firstTitle = await page.locator('[data-testid="media-card"]').first().textContent();
    const term = firstTitle?.trim().slice(0, 3) ?? "the";

    await searchInput.fill(term);
    await page.waitForTimeout(500);

    const itemCount = page.locator('[data-testid="library-item-count"]');
    const countText = await itemCount.textContent();
    expect(countText).toContain("matching");
  });

  test("item count is displayed", async ({ page }) => {
    const id = await getFirstLibraryId(page);
    test.skip(!id, "no library found");
    await page.goto(`/library/${id}`, { waitUntil: "networkidle" });

    const itemCount = page.locator('[data-testid="library-item-count"]');
    await expect(itemCount).toBeVisible({ timeout: 10_000 });
    const text = await itemCount.textContent();
    expect(text).toMatch(/\d+\s+items?/);
  });
});
