import { test, expect } from "./fixtures/auth";

test.describe("Admin libraries", () => {
  test("loads with heading and add button", async ({ page }) => {
    await page.goto("/admin/libraries", { waitUntil: "networkidle" });

    await expect(
      page.locator("h1", { hasText: "Libraries" }),
    ).toBeVisible({ timeout: 10_000 });

    await expect(
      page.locator("button", { hasText: "Add Library" }),
    ).toBeVisible();
  });

  test("shows library table headers", async ({ page }) => {
    await page.goto("/admin/libraries", { waitUntil: "networkidle" });

    const headers = ["Name", "Type", "Paths", "Items", "Last Scanned", "Actions"];
    for (const h of headers) {
      await expect(
        page.locator("th", { hasText: h }),
      ).toBeVisible({ timeout: 5_000 });
    }
  });

  test("add library button opens modal", async ({ page }) => {
    await page.goto("/admin/libraries", { waitUntil: "networkidle" });

    await page.click("button:has-text('Add Library')");

    await expect(
      page.locator("text=Add Library").last(),
    ).toBeVisible({ timeout: 5_000 });
  });
});
