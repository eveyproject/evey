import { test, expect } from "./fixtures/auth";

test.describe("Notifications", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/home", { waitUntil: "networkidle" });
  });

  test("notification bell is visible in TopBar", async ({ page }) => {
    const bell = page.getByRole("button", { name: "Notifications" });
    await expect(bell).toBeVisible({ timeout: 10_000 });
  });

  test("clicking bell opens dropdown", async ({ page }) => {
    const bell = page.getByRole("button", { name: "Notifications" });
    await expect(bell).toBeVisible({ timeout: 10_000 });
    await bell.click();

    const dropdownHeader = page.getByText("Notifications", { exact: true }).last();
    await expect(dropdownHeader).toBeVisible({ timeout: 5_000 });
  });

  test("dropdown shows notifications or empty state", async ({ page }) => {
    const bell = page.getByRole("button", { name: "Notifications" });
    await bell.click();

    const emptyState = page.getByText("No notifications");
    const notifItem = page.locator(".max-h-80 > div").first();
    // Either empty state or at least one notification
    await expect(emptyState.or(notifItem)).toBeVisible({ timeout: 5_000 });
  });

  test("dropdown has View all link", async ({ page }) => {
    const bell = page.getByRole("button", { name: "Notifications" });
    await bell.click();

    const viewAll = page.getByText("View all notifications");
    await expect(viewAll).toBeVisible({ timeout: 5_000 });
  });

  test("clicking outside closes dropdown", async ({ page }) => {
    const bell = page.getByRole("button", { name: "Notifications" });
    await bell.click();

    const dropdownHeader = page.getByText("Notifications", { exact: true }).last();
    await expect(dropdownHeader).toBeVisible({ timeout: 5_000 });

    // Click on page body (outside dropdown)
    await page.locator("header").click({ position: { x: 10, y: 10 } });
    await page.waitForTimeout(500);

    // Dropdown should be gone — "View all notifications" link is inside
    const viewAll = page.getByText("View all notifications");
    await expect(viewAll).not.toBeVisible({ timeout: 3_000 });
  });
});
