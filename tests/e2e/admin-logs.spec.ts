import { test, expect } from "./fixtures/auth";

test.describe("Admin logs", () => {
  test("loads with heading", async ({ page }) => {
    await page.goto("/admin/logs", { waitUntil: "networkidle" });

    await expect(
      page.locator("h2", { hasText: "Live Logs" }),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("shows coming soon message", async ({ page }) => {
    await page.goto("/admin/logs", { waitUntil: "networkidle" });

    await expect(
      page.locator("text=Coming soon"),
    ).toBeVisible({ timeout: 10_000 });
  });
});
