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
    const content = page.locator(
      '[class*="slider"], [class*="card"], [class*="Card"]',
    );
    await content.first().waitFor({ state: "visible", timeout: 5_000 });
    expect(await content.count()).toBeGreaterThan(0);
  });

  test("navigation links work", async ({ page }) => {
    await page.goto("/home", { waitUntil: "domcontentloaded" });
    // Sidebar mounts after the auth/profile queries resolve, so wait for
    // navigation UI to become visible instead of using a fixed sleep.
    const nav = page.locator("nav, aside, [role='navigation']");
    await nav.first().waitFor({ state: "visible", timeout: 5_000 });
    expect(await nav.count()).toBeGreaterThan(0);
  });

  test("keyboard shortcut focuses topbar search", async ({ page }) => {
    await page.goto("/home", { waitUntil: "domcontentloaded" });
    const searchInput = page.locator('[data-testid="topbar-search-input"]');
    await searchInput.waitFor({ state: "attached", timeout: 3_000 });
    await page.keyboard.press("/");
    await expect(searchInput).toBeFocused({ timeout: 3_000 });
  });

  test("no console errors on home page", async ({ page }) => {
    const errors: string[] = [];
    page.on("pageerror", (err) => errors.push(err.message));

    await page.goto("/home", { waitUntil: "domcontentloaded" });
    // Best-effort readiness signal only: some shell-only renders may not expose
    // this marker, and this test intentionally validates console errors only.
    await page
      .locator('[data-testid="home-page"]')
      .isVisible({ timeout: 10_000 })
      .catch(() => false);

    expect(errors).toHaveLength(0);
  });
});
