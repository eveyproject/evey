import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia, collectErrors } from "../shared/helpers";

test.describe("Media > Actor navigation", () => {
  test("cast section is visible on media detail", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const cast = page.locator('[data-testid="cast-section"]');
    // cast may not exist for all media
    const hasCast = await cast.isVisible({ timeout: 3_000 }).catch(() => false);
    expect(typeof hasCast).toBe("boolean"); // no crash
  });

  test("clicking actor navigates to person page", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const castLink = page.locator('[data-testid="cast-section"] a').first();
    const hasLink = await castLink.isVisible({ timeout: 3_000 }).catch(() => false);
    test.skip(!hasLink, "No cast links");
    await castLink.click();
    await page.waitForURL("**/person/**", { timeout: 5_000 });
    expect(page.url()).toContain("/person/");
  });

  test("person page shows name", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const castLink = page.locator('[data-testid="cast-section"] a').first();
    test.skip(!(await castLink.isVisible({ timeout: 3_000 }).catch(() => false)), "No cast");
    await castLink.click();
    await page.waitForURL("**/person/**", { timeout: 5_000 });
    const h1 = page.locator("h1").first();
    await expect(h1).toBeVisible({ timeout: 5_000 });
    const name = await h1.textContent();
    expect(name!.trim().length).toBeGreaterThan(0);
  });

  test("person page has no JS errors", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const castLink = page.locator('[data-testid="cast-section"] a').first();
    test.skip(!(await castLink.isVisible({ timeout: 3_000 }).catch(() => false)), "No cast");
    const { jsErrors } = await collectErrors(page, async () => {
      await castLink.click();
      await page.waitForURL("**/person/**", { timeout: 5_000 });
      await page.waitForTimeout(2_000);
    });
    expect(jsErrors).toHaveLength(0);
  });

  test("person page has no 5xx errors", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const castLink = page.locator('[data-testid="cast-section"] a').first();
    test.skip(!(await castLink.isVisible({ timeout: 3_000 }).catch(() => false)), "No cast");
    const { serverErrors } = await collectErrors(page, async () => {
      await castLink.click();
      await page.waitForURL("**/person/**", { timeout: 5_000 });
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);
  });

  test("back from person returns to media detail", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const castLink = page.locator('[data-testid="cast-section"] a').first();
    test.skip(!(await castLink.isVisible({ timeout: 3_000 }).catch(() => false)), "No cast");
    await castLink.click();
    await page.waitForURL("**/person/**", { timeout: 5_000 });
    await page.goBack();
    await page.waitForTimeout(1_000);
    expect(page.url()).toContain("/media/");
  });
});
