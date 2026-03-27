import { test, expect } from "../../fixtures/auth";
import { goto, gotoRandomMedia, collectErrors } from "../shared/helpers";

test.describe("Media > Navigation flow", () => {
  test("home → card click → media detail → back → home", async ({ page }) => {
    await goto(page, "/home");
    await page.waitForSelector('[data-testid="media-card"]', { timeout: 10_000 });
    await page.locator('[data-testid="media-card"]').first().click();
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    expect(page.url()).toContain("/media/");
    await page.goBack();
    await page.waitForSelector('[data-testid="home-page"]', { timeout: 5_000 });
  });

  test("search → result click → media detail", async ({ page }) => {
    await goto(page, "/home");
    await page.fill('[data-testid="topbar-search-input"]', "the");
    await page.waitForSelector("[data-search-item]", { timeout: 5_000 });
    await page.locator("[data-search-item]").first().click();
    await page.waitForTimeout(2_000);
    expect(page.url()).toContain("/media/");
  });

  test("media detail → cast person → filmography card → media detail", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media");
    const castLink = page.locator('[data-testid="cast-section"] a').first();
    test.skip(!(await castLink.isVisible({ timeout: 3_000 }).catch(() => false)), "No cast");
    await castLink.click();
    await page.waitForURL("**/person/**", { timeout: 5_000 });
    // click first filmography card
    const filmoCard = page.locator('[data-testid="media-card"]').first();
    if (await filmoCard.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await filmoCard.click();
      await page.waitForTimeout(2_000);
      expect(page.url()).toContain("/media/");
    }
  });

  test("library → card → media detail → play → back → media detail", async ({ page }) => {
    await goto(page, "/library");
    await page.waitForTimeout(3_000);
    const card = page.locator('[data-testid="media-card"]').first();
    test.skip(!(await card.isVisible({ timeout: 3_000 }).catch(() => false)), "No cards in library");
    await card.click();
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    const url1 = page.url();
    const playBtn = page.locator('[data-testid="play-button"]');
    if (await playBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await playBtn.click();
      await page.waitForURL("**/player/**", { timeout: 10_000 });
      await page.goBack();
      await page.waitForTimeout(2_000);
      expect(page.url()).toContain("/media/");
    }
  });

  test("full navigation has no JS errors", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await goto(page, "/home");
      await page.waitForSelector('[data-testid="media-card"]', { timeout: 10_000 });
      await page.locator('[data-testid="media-card"]').first().click();
      await page.waitForTimeout(2_000);
      await page.goBack();
      await page.waitForTimeout(1_000);
      await goto(page, "/favorites");
      await page.waitForTimeout(1_000);
      await goto(page, "/library");
      await page.waitForTimeout(1_000);
    });
    expect(jsErrors).toHaveLength(0);
  });
});
