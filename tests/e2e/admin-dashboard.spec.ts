import { test, expect } from "./fixtures/auth";

test.describe("Admin dashboard", () => {
  test("loads with heading and nav links", async ({ page }) => {
    await page.goto("/admin", { waitUntil: "networkidle" });

    await expect(
      page.locator("h1", { hasText: "Dashboard" }),
    ).toBeVisible({ timeout: 10_000 });

    await expect(
      page.locator('a[href="/admin/libraries"]').first(),
    ).toBeVisible();
  });

  test("shows system info card", async ({ page }) => {
    await page.goto("/admin", { waitUntil: "networkidle" });

    await expect(
      page.locator("h3", { hasText: "System Info" }),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("admin nav contains all sections", async ({ page }) => {
    await page.goto("/admin", { waitUntil: "networkidle" });

    const labels = [
      "Libraries", "Users", "Plugins", "Webhooks",
      "Audit Log", "Metadata", "Activity", "Tasks", "Logs",
    ];

    for (const label of labels) {
      await expect(
        page.locator("a", { hasText: label }).first(),
      ).toBeVisible({ timeout: 5_000 });
    }
  });
});
