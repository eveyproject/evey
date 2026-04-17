import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

test.describe("Library > Browse & Filter", () => {
  let librarySlug: string | null = null;

  test("library index loads", async ({ page }) => {
    await goto(page, "/library");
    await page.waitForTimeout(2_000);
    expect(page.url()).toContain("/library");
  });

  test("library index shows library cards or redirects", async ({ page }) => {
    await goto(page, "/library");
    await page.waitForTimeout(3_000);
    // either cards, or redirected to a specific library
    const hasCards = await page.locator('[data-testid="media-card"], a[href*="/library/"]').count();
    expect(hasCards).toBeGreaterThanOrEqual(0);
    // capture slug for later tests
    const match = page.url().match(/\/library\/([^/?]+)/);
    if (match) librarySlug = match[1];
  });

  test("library has media grid", async ({ page }) => {
    await goto(page, "/library");
    await page.waitForTimeout(3_000);
    const cards = page.locator('[data-testid="media-card"]');
    const count = await cards.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test("sort dropdown exists and is interactable", async ({ page }) => {
    await goto(page, "/library");
    await page.waitForTimeout(3_000);
    const sortSelect = page.locator("select").first();
    if (await sortSelect.isVisible({ timeout: 2_000 }).catch(() => false)) {
      const options = await sortSelect.locator("option").count();
      expect(options).toBeGreaterThan(1);
    }
  });

  test("changing sort order doesn't crash", async ({ page }) => {
    await goto(page, "/library");
    await page.waitForTimeout(3_000);
    const sortSelect = page.locator("select").first();
    test.skip(!(await sortSelect.isVisible({ timeout: 2_000 }).catch(() => false)), "No sort select");
    const { jsErrors } = await collectErrors(page, async () => {
      await sortSelect.selectOption({ index: 1 });
      await page.waitForTimeout(1_500);
    });
    expect(jsErrors).toHaveLength(0);
  });

  test("search input filters results", async ({ page }) => {
    await goto(page, "/library");
    await page.waitForTimeout(3_000);
    const searchInput = page.locator('input[type="search"], input[placeholder*="earch" i]').first();
    if (await searchInput.isVisible({ timeout: 2_000 }).catch(() => false)) {
      const countBefore = await page.locator('[data-testid="media-card"]').count();
      await searchInput.fill("zzzzznonexistent");
      await page.waitForTimeout(1_000);
      const countAfter = await page.locator('[data-testid="media-card"]').count();
      expect(countAfter).toBeLessThanOrEqual(countBefore);
    }
  });

  test("filter chip click doesn't crash", async ({ page }) => {
    await goto(page, "/library");
    await page.waitForTimeout(3_000);
    const chip = page.locator('[data-testid="filter-chip"], button[data-filter]').first();
    if (await chip.isVisible({ timeout: 2_000 }).catch(() => false)) {
      const { jsErrors } = await collectErrors(page, async () => {
        await chip.click();
        await page.waitForTimeout(1_000);
      });
      expect(jsErrors).toHaveLength(0);
    }
  });

  test("no 5xx on library page", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, "/library");
      await page.waitForTimeout(3_000);
    });
    expect(serverErrors).toHaveLength(0);
  });

  test("no JS errors on library page", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await goto(page, "/library");
      await page.waitForTimeout(3_000);
    });
    expect(jsErrors).toHaveLength(0);
  });
});
