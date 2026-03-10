import { test, expect } from "./fixtures/auth";

test.describe("Profile notifications", () => {
  test("loads with heading", async ({ page }) => {
    await page.goto("/profile/notifications", { waitUntil: "networkidle" });

    await expect(
      page.locator("h1", { hasText: "Notifications" }),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("shows notifications list or empty state", async ({ page }) => {
    await page.goto("/profile/notifications", { waitUntil: "networkidle" });

    const hasList = await page.locator("[class*='surface-raised']").count();
    const hasEmpty = await page.locator("text=No notifications").count();

    expect(hasList + hasEmpty).toBeGreaterThan(0);
  });
});
