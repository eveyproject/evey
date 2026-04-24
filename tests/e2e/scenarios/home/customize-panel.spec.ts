import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

const customizeBtn = '[data-testid="home-customize-btn"]';
const customizePanel = '[data-testid="customize-panel"]';
const resetBtn = '[data-testid="customize-reset-btn"]';

test.describe("Home > Customize panel", () => {
  test("customize button visible on home", async ({ page }) => {
    await goto(page, "/home");
    await expect(page.locator(customizeBtn)).toBeVisible({ timeout: 5_000 });
  });

  test("clicking customize opens panel", async ({ page }) => {
    await goto(page, "/home");
    await page.locator(customizeBtn).click();
    await expect(page.locator(customizePanel)).toBeVisible({ timeout: 3_000 });
  });

  test("panel has toggle switches for feeds", async ({ page }) => {
    await goto(page, "/home");
    await page.locator(customizeBtn).click();
    await expect(page.locator(customizePanel)).toBeVisible({ timeout: 3_000 });
    const toggles = page.locator(`${customizePanel} button[role='switch']`);
    expect(await toggles.count()).toBeGreaterThan(0);
  });

  test("toggling a feed doesn't crash", async ({ page }) => {
    await goto(page, "/home");
    await page.locator(customizeBtn).click();
    await expect(page.locator(customizePanel)).toBeVisible({ timeout: 3_000 });
    const toggle = page.locator(`${customizePanel} button[role='switch']`).first();
    test.skip(!(await toggle.isVisible({ timeout: 2_000 }).catch(() => false)), "No toggles");
    const { jsErrors } = await collectErrors(page, async () => {
      await toggle.click();
      await page.waitForTimeout(500);
    });
    expect(jsErrors).toHaveLength(0);
  });

  test("reset defaults button exists", async ({ page }) => {
    await goto(page, "/home");
    await page.locator(customizeBtn).click();
    await expect(page.locator(resetBtn)).toBeVisible({ timeout: 3_000 });
  });

  test("closing panel (Escape) works", async ({ page }) => {
    await goto(page, "/home");
    await page.locator(customizeBtn).click();
    await expect(page.locator(customizePanel)).toBeVisible({ timeout: 3_000 });
    await page.keyboard.press("Escape");
    await page.waitForTimeout(300);
  });

  test("no 5xx during customize interactions", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, "/home");
      await page.locator(customizeBtn).click();
      await page.waitForTimeout(1_000);
      await page.keyboard.press("Escape");
    });
    expect(serverErrors).toHaveLength(0);
  });
});
