import { test, expect } from "./fixtures/auth";

test.describe("Admin activity", () => {
  test("loads with heading and tabs", async ({ page }) => {
    await page.goto("/admin/activity", { waitUntil: "networkidle" });

    await expect(
      page.locator("h1", { hasText: "Activity" }),
    ).toBeVisible({ timeout: 10_000 });

    await expect(
      page.locator("button", { hasText: "Play History" }),
    ).toBeVisible();
    await expect(
      page.locator("button", { hasText: "Login History" }),
    ).toBeVisible();
    await expect(
      page.locator("button", { hasText: "Server Stats" }),
    ).toBeVisible();
  });

  test("play history tab shows table", async ({ page }) => {
    await page.goto("/admin/activity", { waitUntil: "networkidle" });

    // play history is default tab
    await expect(
      page.locator("th", { hasText: "Title" }),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("login history tab loads", async ({ page }) => {
    await page.goto("/admin/activity", { waitUntil: "networkidle" });

    await page.click("button:has-text('Login History')");

    await expect(
      page.locator("th", { hasText: "IP" }),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("server stats tab loads", async ({ page }) => {
    await page.goto("/admin/activity", { waitUntil: "networkidle" });

    await page.click("button:has-text('Server Stats')");

    await expect(
      page.locator("text=Total Plays"),
    ).toBeVisible({ timeout: 10_000 });
  });
});
