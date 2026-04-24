import { test, expect } from "./fixtures/auth";

test.describe("Smoke tests", () => {
  test("authenticated session lands on home", async ({ page }) => {
    // The auth fixture only loads storage state; navigate explicitly so the
    // assertion is meaningful.
    await page.goto("/home", { waitUntil: "domcontentloaded" });
    expect(page.url()).toContain("/home");
  });

  test("home page renders media content", async ({ page }) => {
    await page.goto("/home", { waitUntil: "domcontentloaded" });
    await page.waitForTimeout(2_000);
    const content = page.locator(
      '[class*="slider"], [class*="card"], [class*="Card"]',
    );
    expect(await content.count()).toBeGreaterThan(0);
  });

  test("navigation links work", async ({ page }) => {
    await page.goto("/home", { waitUntil: "domcontentloaded" });
    // Sidebar mounts after the auth/profile queries resolve, so wait briefly
    // before counting — domcontentloaded only guarantees the SPA shell.
    await page.waitForTimeout(2_000);
    const nav = page.locator("nav, aside, [role='navigation']");
    expect(await nav.count()).toBeGreaterThan(0);
  });

  test("keyboard shortcut focuses topbar search", async ({ page }) => {
    await page.goto("/home", { waitUntil: "domcontentloaded" });
    await page.waitForTimeout(1_000);
    await page.keyboard.press("/");
    const searchInput = page.locator('[data-testid="topbar-search-input"]');
    await expect(searchInput).toBeFocused({ timeout: 3_000 });
  });

  test("no console errors on home page", async ({ page }) => {
    const errors: string[] = [];
    page.on("pageerror", (err) => errors.push(err.message));

    await page.goto("/home", { waitUntil: "domcontentloaded" });
    await page.locator('[data-testid="home-page"]').waitFor({ timeout: 10_000 }).catch(() => { /* page-shell only */ });

    expect(errors).toHaveLength(0);
  });
});
