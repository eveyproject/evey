import { test, expect } from "./fixtures/auth";

test.describe("Profile page", () => {
  test("loads with heading and user info", async ({ page }) => {
    await page.goto("/profile", { waitUntil: "networkidle" });

    await expect(
      page.locator("h1", { hasText: "Profile" }),
    ).toBeVisible({ timeout: 10_000 });

    // user avatar initial
    await expect(
      page.locator("text=gravity").first(),
    ).toBeVisible();
  });

  test("shows edit button", async ({ page }) => {
    await page.goto("/profile", { waitUntil: "networkidle" });

    await expect(
      page.locator("button", { hasText: "Edit" }),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("shows nav links to settings, history, notifications", async ({ page }) => {
    await page.goto("/profile", { waitUntil: "networkidle" });

    await expect(page.locator("text=Settings").first()).toBeVisible({ timeout: 10_000 });
    await expect(page.locator("text=Watch History").first()).toBeVisible();
    await expect(page.locator("text=Notifications").first()).toBeVisible();
  });

  test("shows admin panel link for admin user", async ({ page }) => {
    await page.goto("/profile", { waitUntil: "networkidle" });

    await expect(
      page.locator("text=Admin Panel"),
    ).toBeVisible({ timeout: 10_000 });
  });
});
