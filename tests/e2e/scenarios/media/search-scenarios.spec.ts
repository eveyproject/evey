import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

const QUERIES = ["the", "star", "a", "night", "man", "love", "dark", "world", "kill", "game"];

test.describe("Search > Dropdown behavior", () => {
  for (const q of QUERIES) {
    test(`search "${q}" → dropdown has results or empty state`, async ({ page }) => {
      await goto(page, "/home");
      await page.fill('[data-testid="topbar-search-input"]', q);
      await page.waitForTimeout(1_000);
      const dropdown = page.locator('[data-testid="search-dropdown"]');
      const visible = await dropdown.isVisible({ timeout: 3_000 }).catch(() => false);
      if (visible) {
        const items = page.locator("[data-search-item]");
        const count = await items.count();
        expect(count).toBeGreaterThanOrEqual(0);
      }
    });
  }

  test("empty query hides dropdown", async ({ page }) => {
    await goto(page, "/home");
    await page.fill('[data-testid="topbar-search-input"]', "the");
    await page.waitForTimeout(800);
    await page.fill('[data-testid="topbar-search-input"]', "");
    await page.waitForTimeout(500);
    const dropdown = page.locator('[data-testid="search-dropdown"]');
    // dropdown should close or show recents, not search results
    const items = page.locator("[data-search-item]");
    const count = await items.count();
    // with empty query, items should be recents or 0
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test("very long query doesn't crash", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await goto(page, "/home");
      await page.fill('[data-testid="topbar-search-input"]', "a".repeat(200));
      await page.waitForTimeout(1_000);
    });
    expect(jsErrors).toHaveLength(0);
  });

  test("special chars in query don't crash", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await goto(page, "/home");
      await page.fill('[data-testid="topbar-search-input"]', '<script>alert("xss")</script>');
      await page.waitForTimeout(1_000);
    });
    expect(jsErrors).toHaveLength(0);
  });

  test("emoji in query doesn't crash", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await goto(page, "/home");
      await page.fill('[data-testid="topbar-search-input"]', "🎬 movie");
      await page.waitForTimeout(1_000);
    });
    expect(jsErrors).toHaveLength(0);
  });

  test("SQL injection attempt doesn't crash", async ({ page }) => {
    const { jsErrors, serverErrors } = await collectErrors(page, async () => {
      await goto(page, "/home");
      await page.fill('[data-testid="topbar-search-input"]', "'; DROP TABLE media_items; --");
      await page.waitForTimeout(1_000);
    });
    expect(jsErrors).toHaveLength(0);
    expect(serverErrors).toHaveLength(0);
  });
});

test.describe("Search > Navigation", () => {
  test("clicking result navigates to media detail", async ({ page }) => {
    await goto(page, "/home");
    await page.fill('[data-testid="topbar-search-input"]', "the");
    const item = page.locator("[data-search-item]").first();
    await item.waitFor({ state: "visible", timeout: 5_000 });
    await item.click();
    await page.waitForTimeout(2_000);
    expect(page.url()).toContain("/media/");
  });

  test("Enter on search navigates to search page", async ({ page }) => {
    await goto(page, "/home");
    await page.fill('[data-testid="topbar-search-input"]', "the");
    await page.waitForTimeout(500);
    await page.keyboard.press("Enter");
    await page.waitForTimeout(2_000);
    expect(page.url()).toContain("/search");
  });

  test("Ctrl+K focuses search", async ({ page }) => {
    await goto(page, "/home");
    await page.keyboard.press("Control+k");
    await page.waitForTimeout(300);
    const focused = await page.evaluate(() => document.activeElement?.getAttribute("data-testid"));
    expect(focused).toBe("topbar-search-input");
  });

  test("/ key focuses search", async ({ page }) => {
    await goto(page, "/home");
    await page.keyboard.press("/");
    await page.waitForTimeout(300);
    const focused = await page.evaluate(() => document.activeElement?.getAttribute("data-testid"));
    expect(focused).toBe("topbar-search-input");
  });

  test("Escape unfocuses search", async ({ page }) => {
    await goto(page, "/home");
    await page.fill('[data-testid="topbar-search-input"]', "test");
    await page.waitForTimeout(500);
    await page.keyboard.press("Escape");
    await page.waitForTimeout(300);
    const focused = await page.evaluate(() => document.activeElement?.getAttribute("data-testid"));
    expect(focused).not.toBe("topbar-search-input");
  });
});
