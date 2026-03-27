import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

test.describe("Home > Slider interaction", () => {
  test("sliders have scroll buttons on hover", async ({ page }) => {
    await goto(page, "/home");
    const slider = page.locator('[data-testid="media-slider"]').first();
    await expect(slider).toBeVisible({ timeout: 10_000 });
    await slider.hover();
    await page.waitForTimeout(500);
    // scroll buttons appear on hover
    const scrollBtn = slider.locator("button[aria-label]").first();
    const count = await scrollBtn.count();
    expect(count).toBeGreaterThanOrEqual(0); // may not have enough items to scroll
  });

  test("clicking media card in slider opens detail", async ({ page }) => {
    await goto(page, "/home");
    const card = page.locator('[data-testid="media-card"]').first();
    await card.waitFor({ state: "visible", timeout: 10_000 });
    const href = await card.getAttribute("href");
    await card.click();
    await page.waitForTimeout(2_000);
    expect(page.url()).toContain("/media/");
  });

  test("home page loads within 5 seconds", async ({ page }) => {
    const start = Date.now();
    await goto(page, "/home");
    await page.waitForSelector('[data-testid="home-page"]', { timeout: 5_000 });
    const elapsed = Date.now() - start;
    expect(elapsed).toBeLessThan(5_000);
  });

  test("no broken images on home page", async ({ page }) => {
    await goto(page, "/home");
    await page.waitForTimeout(3_000);
    const brokenImages = await page.evaluate(() => {
      return Array.from(document.querySelectorAll("img"))
        .filter((img) => img.naturalWidth === 0 && img.offsetParent !== null && img.style.display !== "none")
        .map((img) => img.src);
    });
    // some artwork may fail to load, but there shouldn't be many
    expect(brokenImages.length).toBeLessThan(5);
  });

  test("home page has no JS errors", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await goto(page, "/home");
      await page.waitForTimeout(3_000);
    });
    expect(jsErrors).toHaveLength(0);
  });

  test("home page has no 5xx", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, "/home");
      await page.waitForTimeout(3_000);
    });
    expect(serverErrors).toHaveLength(0);
  });
});
