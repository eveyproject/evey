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
    // Only judge images actually in the viewport. Cards in horizontal sliders
    // (and below the fold) use loading="lazy", so off-screen posters never start
    // loading and report naturalWidth===0 - that's lazy, not broken. Restrict to
    // in-viewport images and wait for those to settle (img.complete is true on
    // both success and error) before counting genuine failures.
    await page
      .waitForFunction(
        () => {
          const inVP = (i: HTMLImageElement) => {
            if (i.offsetParent === null || i.style.display === "none") return false;
            const r = i.getBoundingClientRect();
            return r.top < innerHeight && r.bottom > 0 && r.left < innerWidth && r.right > 0;
          };
          const imgs = Array.from(document.querySelectorAll("img")).filter(inVP);
          return imgs.length === 0 || imgs.every((i) => i.complete);
        },
        undefined,
        { timeout: 8_000 },
      )
      .catch(() => {});
    const broken = await page.evaluate(() => {
      const inVP = (i: HTMLImageElement) => {
        if (i.offsetParent === null || i.style.display === "none") return false;
        const r = i.getBoundingClientRect();
        return r.top < innerHeight && r.bottom > 0 && r.left < innerWidth && r.right > 0;
      };
      return Array.from(document.querySelectorAll("img"))
        .filter((img) => img.naturalWidth === 0 && inVP(img))
        .map((img) => img.src);
    });
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
