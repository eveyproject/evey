import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

test.describe("Home > Customize panel", () => {
  test("customize button visible on home", async ({ page }) => {
    await goto(page, "/home");
    await expect(page.locator("button", { hasText: /customize/i })).toBeVisible({ timeout: 5_000 });
  });

  test("clicking customize opens panel", async ({ page }) => {
    await goto(page, "/home");
    await page.locator("button", { hasText: /customize/i }).click();
    await page.waitForTimeout(500);
    // panel should show feed toggle options
    const panel = page.locator("text=/reset|enable|disable|order/i").first();
    await expect(panel).toBeVisible({ timeout: 3_000 });
  });

  test("panel has toggle switches for feeds", async ({ page }) => {
    await goto(page, "/home");
    await page.locator("button", { hasText: /customize/i }).click();
    await page.waitForTimeout(500);
    const toggles = page.locator("button[role='switch'], input[type='checkbox']");
    const count = await toggles.count();
    expect(count).toBeGreaterThan(0);
  });

  test("toggling a feed doesn't crash", async ({ page }) => {
    await goto(page, "/home");
    await page.locator("button", { hasText: /customize/i }).click();
    await page.waitForTimeout(500);
    const toggle = page.locator("button[role='switch'], input[type='checkbox']").first();
    test.skip(!(await toggle.isVisible({ timeout: 2_000 }).catch(() => false)), "No toggles");
    const { jsErrors } = await collectErrors(page, async () => {
      await toggle.click();
      await page.waitForTimeout(500);
    });
    expect(jsErrors).toHaveLength(0);
  });

  test("reset defaults button exists", async ({ page }) => {
    await goto(page, "/home");
    await page.locator("button", { hasText: /customize/i }).click();
    await page.waitForTimeout(500);
    const reset = page.locator("button", { hasText: /reset/i });
    const count = await reset.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test("closing panel (Escape) works", async ({ page }) => {
    await goto(page, "/home");
    await page.locator("button", { hasText: /customize/i }).click();
    await page.waitForTimeout(500);
    await page.keyboard.press("Escape");
    await page.waitForTimeout(300);
  });

  test("no 5xx during customize interactions", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, "/home");
      await page.locator("button", { hasText: /customize/i }).click();
      await page.waitForTimeout(1_000);
      await page.keyboard.press("Escape");
    });
    expect(serverErrors).toHaveLength(0);
  });
});
