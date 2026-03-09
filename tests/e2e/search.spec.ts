import { test, expect } from "./fixtures/auth";

test.describe("Search", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/home", { waitUntil: "networkidle" });
  });

  test("search input is visible in TopBar", async ({ page }) => {
    const input = page.locator('[data-testid="topbar-search-input"]');
    await expect(input).toBeVisible();
  });

  test("Ctrl+K focuses search input", async ({ page }) => {
    const input = page.locator('[data-testid="topbar-search-input"]');
    await page.keyboard.press("Control+k");
    await expect(input).toBeFocused({ timeout: 3_000 });
  });

  test("typing 2+ chars shows dropdown with results", async ({ page }) => {
    const input = page.locator('[data-testid="topbar-search-input"]');
    await input.click();
    await input.fill("the");

    const dropdown = page.locator('[data-testid="search-dropdown"]');
    await expect(dropdown).toBeVisible({ timeout: 10_000 });

    const items = dropdown.locator("[data-search-item]");
    const count = await items.count();
    expect(count).toBeGreaterThan(0);
  });

  test("dropdown results show title and type badge", async ({ page }) => {
    const input = page.locator('[data-testid="topbar-search-input"]');
    await input.click();
    await input.fill("the");

    const dropdown = page.locator('[data-testid="search-dropdown"]');
    await expect(dropdown).toBeVisible({ timeout: 10_000 });

    const firstItem = dropdown.locator("[data-search-item]").first();
    const text = await firstItem.textContent();
    expect(text).toBeTruthy();
    expect(text!.length).toBeGreaterThan(0);
  });

  test("keyboard nav: ArrowDown selects item, Enter navigates", async ({ page }) => {
    const input = page.locator('[data-testid="topbar-search-input"]');
    await input.click();
    await input.fill("the");

    const dropdown = page.locator('[data-testid="search-dropdown"]');
    await expect(dropdown).toBeVisible({ timeout: 10_000 });

    await page.keyboard.press("ArrowDown");
    const focused = dropdown.locator("[data-search-item]:focus");
    await expect(focused).toBeVisible({ timeout: 3_000 });

    await page.keyboard.press("Enter");
    await page.waitForURL(/\/media\//, { timeout: 10_000 });
    expect(page.url()).toContain("/media/");
  });

  test("Escape closes dropdown", async ({ page }) => {
    const input = page.locator('[data-testid="topbar-search-input"]');
    await input.click();
    await input.fill("the");

    const dropdown = page.locator('[data-testid="search-dropdown"]');
    await expect(dropdown).toBeVisible({ timeout: 10_000 });

    await page.keyboard.press("Escape");
    await expect(dropdown).not.toBeVisible({ timeout: 3_000 });
  });

  test("clicking a result navigates to media page", async ({ page }) => {
    const input = page.locator('[data-testid="topbar-search-input"]');
    await input.click();
    await input.fill("the");

    const dropdown = page.locator('[data-testid="search-dropdown"]');
    await expect(dropdown).toBeVisible({ timeout: 10_000 });

    const firstItem = dropdown.locator("[data-search-item]").first();
    await firstItem.click();
    await page.waitForURL(/\/media\//, { timeout: 10_000 });
    expect(page.url()).toContain("/media/");
  });

  test("gibberish search shows no results in dropdown", async ({ page }) => {
    const input = page.locator('[data-testid="topbar-search-input"]');
    await input.click();
    await input.fill("zzzxxxqqqnonsense");

    const dropdown = page.locator('[data-testid="search-dropdown"]');
    await expect(dropdown).toBeVisible({ timeout: 10_000 });

    const empty = dropdown.locator('[data-testid="search-dropdown-empty"]');
    await expect(empty).toBeVisible({ timeout: 5_000 });
  });
});

test.describe("Search page", () => {
  test("input is auto-focused", async ({ page }) => {
    await page.goto("/search", { waitUntil: "networkidle" });
    const input = page.locator('[data-testid="search-page-input"]');
    await expect(input).toBeFocused({ timeout: 5_000 });
  });

  test("type filter tabs appear after search and are clickable", async ({ page }) => {
    await page.goto("/search?q=the", { waitUntil: "networkidle" });

    const filters = page.locator('[data-testid="search-type-filters"]');
    await expect(filters).toBeVisible({ timeout: 10_000 });

    const allBtn = page.locator('[data-testid="search-filter-all"]');
    const moviesBtn = page.locator('[data-testid="search-filter-movies"]');
    const seriesBtn = page.locator('[data-testid="search-filter-series"]');

    await expect(allBtn).toBeVisible();
    await expect(moviesBtn).toBeVisible();
    await expect(seriesBtn).toBeVisible();

    await moviesBtn.click();
    await page.waitForTimeout(500);
    // Filter should be active (styled differently)
    const moviesBg = await moviesBtn.evaluate(
      (el) => getComputedStyle(el).backgroundColor,
    );
    const allBg = await allBtn.evaluate(
      (el) => getComputedStyle(el).backgroundColor,
    );
    expect(moviesBg).not.toBe(allBg);
  });

  test("clear button empties input", async ({ page }) => {
    await page.goto("/search?q=the", { waitUntil: "networkidle" });

    const input = page.locator('[data-testid="search-page-input"]');
    await expect(input).toHaveValue("the", { timeout: 5_000 });

    const clearBtn = page.locator('button[aria-label="Clear search"]');
    await expect(clearBtn).toBeVisible();
    await clearBtn.click();

    await expect(input).toHaveValue("");
  });

  test("empty search shows prompt message", async ({ page }) => {
    await page.goto("/search", { waitUntil: "networkidle" });

    const emptyMsg = page.getByText("Search your media library");
    await expect(emptyMsg).toBeVisible({ timeout: 5_000 });
  });
});
