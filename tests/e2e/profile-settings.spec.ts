import { test, expect } from "./fixtures/auth";

test.describe("Profile settings", () => {
  test("loads with heading and tabs", async ({ page }) => {
    await page.goto("/profile/settings", { waitUntil: "networkidle" });

    await expect(
      page.locator("h1", { hasText: "Settings" }),
    ).toBeVisible({ timeout: 10_000 });

    await expect(page.locator("button", { hasText: "Appearance" })).toBeVisible();
    await expect(page.locator("button", { hasText: "Notifications" })).toBeVisible();
    await expect(page.locator("button", { hasText: "Account" })).toBeVisible();
  });

  test("account tab shows password change form", async ({ page }) => {
    await page.goto("/profile/settings", { waitUntil: "networkidle" });

    await page.click("button:has-text('Account')");

    await expect(
      page.locator("h3", { hasText: "Change Password" }),
    ).toBeVisible({ timeout: 10_000 });

    await expect(
      page.locator("button", { hasText: "Update password" }),
    ).toBeVisible();
  });
});
