import { test, expect } from "./fixtures/auth";

test.describe("Watch history", () => {
  test("loads with heading", async ({ page }) => {
    await page.goto("/profile/history", { waitUntil: "networkidle" });

    await expect(
      page.locator("h1", { hasText: "Watch History" }),
    ).toBeVisible({ timeout: 10_000 });
  });
});
