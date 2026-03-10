import { test, expect } from "./fixtures/auth";

test.describe("Admin metadata", () => {
  test("loads with heading and search", async ({ page }) => {
    await page.goto("/admin/metadata", { waitUntil: "networkidle" });

    await expect(
      page.locator("h1", { hasText: "Metadata" }),
    ).toBeVisible({ timeout: 10_000 });

    await expect(
      page.locator('input[placeholder="Search media items..."]'),
    ).toBeVisible();
  });

  test("shows media grid or empty state", async ({ page }) => {
    await page.goto("/admin/metadata", { waitUntil: "networkidle" });

    const hasGrid = await page.locator("button:has(img)").count();
    const hasEmpty = await page.locator("text=No media items found").count();

    expect(hasGrid + hasEmpty).toBeGreaterThan(0);
  });
});
