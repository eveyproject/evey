import { test, expect } from "./fixtures/auth";

test.describe("Smoke tests", () => {
  test("login redirects to home", async ({ page }) => {
    // auth fixture already logged in
    expect(page.url()).toContain("/home");
  });

  test("home page renders media content", async ({ page }) => {
    await page.goto("/home", { waitUntil: "networkidle" });
    // should have at least one slider row or media card
    const content = page.locator(
      '[class*="slider"], [class*="card"], [class*="Card"]',
    );
    const count = await content.count();
    expect(count).toBeGreaterThan(0);
  });

  test("navigation links work", async ({ page }) => {
    await page.goto("/home", { waitUntil: "networkidle" });

    // check that the page has navigation elements
    const nav = page.locator("nav, aside, [role='navigation']");
    const navCount = await nav.count();
    expect(navCount).toBeGreaterThan(0);
  });

  test("search works", async ({ page }) => {
    await page.goto("/home", { waitUntil: "networkidle" });

    // trigger search with keyboard shortcut
    await page.keyboard.press("/");
    const searchInput = page.locator('input[aria-label="Search"]');
    await expect(searchInput).toBeFocused({ timeout: 3_000 });
  });

  test("no console errors on home page", async ({ page }) => {
    const errors: string[] = [];
    page.on("pageerror", (err) => errors.push(err.message));

    await page.goto("/home", { waitUntil: "networkidle" });
    await page.locator('[data-testid="home-page"]').waitFor({ timeout: 10_000 }).catch(() => {});

    expect(errors).toHaveLength(0);
  });
});
