import { test, expect } from "./fixtures/auth";

test.describe("Admin webhooks", () => {
  test("loads with heading and create button", async ({ page }) => {
    await page.goto("/admin/webhooks", { waitUntil: "networkidle" });

    await expect(
      page.locator("h2", { hasText: "Webhooks" }),
    ).toBeVisible({ timeout: 10_000 });

    await expect(
      page.locator("button", { hasText: "Create Webhook" }),
    ).toBeVisible();
  });

  test("shows webhook table headers", async ({ page }) => {
    await page.goto("/admin/webhooks", { waitUntil: "networkidle" });

    for (const h of ["URL", "Events", "Status", "Created", "Actions"]) {
      await expect(
        page.locator("th", { hasText: h }),
      ).toBeVisible({ timeout: 5_000 });
    }
  });

  test("create webhook modal opens", async ({ page }) => {
    await page.goto("/admin/webhooks", { waitUntil: "networkidle" });

    await page.click("button:has-text('Create Webhook')");

    // modal should appear
    await expect(
      page.locator("text=Create Webhook").last(),
    ).toBeVisible({ timeout: 5_000 });
  });
});
