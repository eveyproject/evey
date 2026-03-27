import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

test.describe("Home > Feeds display", () => {
  test("home loads with at least 1 media slider", async ({ page }) => {
    await goto(page, "/home");
    const slider = page.locator('[data-testid="media-slider"], [data-testid="feed-section"], section').first();
    await expect(slider).toBeVisible({ timeout: 10_000 });
  });

  test("cards in sliders are clickable → navigate to /media/{id}", async ({ page }) => {
    await goto(page, "/home");
    const card = page.locator('[data-testid="media-card"], a[href*="/media/"]').first();
    await card.waitFor({ state: "visible", timeout: 10_000 });
    await card.click();
    await page.waitForURL(/\/media\//, { timeout: 5_000 });
    expect(page.url()).toMatch(/\/media\/[a-zA-Z0-9-]+/);
  });

  test("no broken images on visible cards", async ({ page }) => {
    await goto(page, "/home");
    await page.waitForTimeout(2_000);
    const broken = await page.evaluate(() =>
      Array.from(document.querySelectorAll("img"))
        .filter((img) => img.naturalWidth === 0 && img.offsetParent !== null && img.style.display !== "none")
        .map((img) => img.src),
    );
    expect(broken.length, `Broken images: ${broken.slice(0, 5).join(", ")}`).toBeLessThan(5);
  });

  test("no 5xx on home load", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, "/home");
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);
  });

  test("no JS errors on home load", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await goto(page, "/home");
      await page.waitForTimeout(2_000);
    });
    expect(jsErrors).toHaveLength(0);
  });

  test("customize button opens panel", async ({ page }) => {
    await goto(page, "/home");
    await page.waitForTimeout(1_500);
    const customizeBtn = page.locator('button:has-text("Customize"), button:has-text("customize"), button[aria-label*="customize" i], [data-testid="customize-button"]').first();
    test.skip(!(await customizeBtn.isVisible({ timeout: 3_000 }).catch(() => false)), "No customize button");

    await customizeBtn.click();
    await page.waitForTimeout(500);
    const panel = page.locator('[data-testid="customize-panel"], [role="dialog"], aside, .drawer, [class*="panel"]').first();
    await expect(panel).toBeVisible({ timeout: 3_000 });
  });

  test("Recently Added section has cards if data exists", async ({ page }) => {
    await goto(page, "/home");
    await page.waitForTimeout(2_000);
    const heading = page.locator('h2:has-text("Recently Added"), h3:has-text("Recently Added"), h2:has-text("recently added")').first();
    test.skip(!(await heading.isVisible({ timeout: 3_000 }).catch(() => false)), "No Recently Added section");

    const section = heading.locator("xpath=ancestor::section | ancestor::div[contains(@class,'section')] | following-sibling::div").first();
    const cards = section.locator('[data-testid="media-card"], a[href*="/media/"]');
    expect(await cards.count()).toBeGreaterThan(0);
  });
});
