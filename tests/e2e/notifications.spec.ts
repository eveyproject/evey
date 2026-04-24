import { test, expect } from "./fixtures/auth";

test.describe("Notifications", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/home", { waitUntil: "domcontentloaded" });
  });

  test("notification bell is visible in TopBar", async ({ page }) => {
    await expect(page.locator('[data-testid="notification-bell"]')).toBeVisible({ timeout: 10_000 });
  });

  test("clicking bell opens dropdown", async ({ page }) => {
    const bell = page.locator('[data-testid="notification-bell"]');
    await expect(bell).toBeVisible({ timeout: 10_000 });
    await bell.click();
    await expect(page.locator('[data-testid="notification-dropdown"]')).toBeVisible({ timeout: 5_000 });
  });

  test("dropdown shows the View all link", async ({ page }) => {
    await page.locator('[data-testid="notification-bell"]').click();
    await expect(page.locator('[data-testid="notification-view-all"]')).toBeVisible({ timeout: 5_000 });
  });

  test("clicking outside closes dropdown", async ({ page }) => {
    await page.locator('[data-testid="notification-bell"]').click();
    await expect(page.locator('[data-testid="notification-dropdown"]')).toBeVisible({ timeout: 5_000 });
    await page.locator("header").click({ position: { x: 10, y: 10 } });
    await expect(page.locator('[data-testid="notification-dropdown"]')).not.toBeVisible({ timeout: 3_000 });
  });
});
