import { test, expect } from "./fixtures/auth";

test.describe("Admin audit log", () => {
  test("loads with heading and filters", async ({ page }) => {
    await page.goto("/admin/audit", { waitUntil: "networkidle" });

    await expect(
      page.locator("h1", { hasText: "Audit Log" }),
    ).toBeVisible({ timeout: 10_000 });

    await expect(
      page.locator("label", { hasText: "Action" }),
    ).toBeVisible();

    await expect(
      page.locator("label", { hasText: "Resource Type" }),
    ).toBeVisible();
  });

  test("filter buttons present", async ({ page }) => {
    await page.goto("/admin/audit", { waitUntil: "networkidle" });

    await expect(
      page.locator("button", { hasText: "Apply" }),
    ).toBeVisible({ timeout: 5_000 });
    await expect(
      page.locator("button", { hasText: "Clear" }),
    ).toBeVisible();
  });
});
