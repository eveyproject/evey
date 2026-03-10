import { test, expect } from "./fixtures/auth";

test.describe("Admin scheduled tasks", () => {
  test("loads with heading", async ({ page }) => {
    await page.goto("/admin/tasks", { waitUntil: "networkidle" });

    await expect(
      page.locator("h1", { hasText: "Scheduled Tasks" }),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("shows task cards or empty state", async ({ page }) => {
    await page.goto("/admin/tasks", { waitUntil: "networkidle" });

    const hasCards = await page.locator("button:has-text('Run Now')").count();
    const hasEmpty = await page.locator("text=No scheduled tasks found").count();

    expect(hasCards + hasEmpty).toBeGreaterThan(0);
  });
});
