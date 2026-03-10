import { test, expect } from "./fixtures/auth";

test.describe("Admin logs", () => {
  test("loads with heading and log info", async ({ page }) => {
    await page.goto("/admin/logs", { waitUntil: "networkidle" });

    await expect(
      page.locator("h1", { hasText: "Logs" }),
    ).toBeVisible({ timeout: 10_000 });

    await expect(
      page.locator("h2", { hasText: "Log Location" }),
    ).toBeVisible();
  });

  test("shows log level documentation", async ({ page }) => {
    await page.goto("/admin/logs", { waitUntil: "networkidle" });

    await expect(
      page.locator("h2", { hasText: "Log Levels" }),
    ).toBeVisible({ timeout: 10_000 });

    for (const level of ["INFO", "WARN", "ERROR"]) {
      await expect(
        page.locator(`text=${level}`).first(),
      ).toBeVisible();
    }
  });

  test("shows log rotation info", async ({ page }) => {
    await page.goto("/admin/logs", { waitUntil: "networkidle" });

    await expect(
      page.locator("h2", { hasText: "Log Rotation" }),
    ).toBeVisible({ timeout: 10_000 });
  });
});
