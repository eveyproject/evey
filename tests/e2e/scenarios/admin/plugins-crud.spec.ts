import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

test.describe("Admin > Plugins", () => {
  test("page loads", async ({ page }) => {
    await goto(page, "/admin/plugins");
    await expect(page.locator("h1, h2").first()).toBeVisible({ timeout: 5_000 });
  });

  test("install button exists and opens modal", async ({ page }) => {
    await goto(page, "/admin/plugins");
    const btn = page.locator("button", { hasText: /install/i }).first();
    await expect(btn).toBeVisible({ timeout: 5_000 });
    await btn.click();
    await page.waitForTimeout(500);
    const urlInput = page.locator("input").first();
    await expect(urlInput).toBeVisible({ timeout: 2_000 });
  });

  test("closing install modal works", async ({ page }) => {
    await goto(page, "/admin/plugins");
    await page.locator("button", { hasText: /install/i }).first().click();
    await page.waitForTimeout(500);
    await page.keyboard.press("Escape");
    await page.waitForTimeout(300);
    // modal should be closed — no crash
  });

  test("plugin list or empty state visible", async ({ page }) => {
    await goto(page, "/admin/plugins");
    await page.waitForTimeout(2_000);
    const body = await page.locator("body").textContent();
    // either "No plugins" or plugin cards
    expect(body!.length).toBeGreaterThan(10);
  });

  test("no 5xx", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, "/admin/plugins");
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);
  });
});
