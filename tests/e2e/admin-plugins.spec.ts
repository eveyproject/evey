import { test, expect } from "./fixtures/auth";

test.describe("Admin plugins", () => {
  test("loads with heading and install button", async ({ page }) => {
    await page.goto("/admin/plugins", { waitUntil: "networkidle" });

    await expect(
      page.locator("h2", { hasText: "Plugins" }),
    ).toBeVisible({ timeout: 10_000 });

    await expect(
      page.locator("button", { hasText: "Install Plugin" }),
    ).toBeVisible();
  });

  test("shows plugins or empty message", async ({ page }) => {
    await page.goto("/admin/plugins", { waitUntil: "networkidle" });

    const hasPlugins = await page.locator("[class*='grid'] > div").count();
    const hasEmpty = await page.locator("text=No plugins installed").count();

    expect(hasPlugins + hasEmpty).toBeGreaterThan(0);
  });

  test("install modal opens with form", async ({ page }) => {
    await page.goto("/admin/plugins", { waitUntil: "networkidle" });

    await page.click("button:has-text('Install Plugin')");

    await expect(
      page.locator("label", { hasText: "Source URL" }),
    ).toBeVisible({ timeout: 5_000 });
  });
});
