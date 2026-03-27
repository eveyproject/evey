import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia, collectErrors } from "../shared/helpers";

const VIEWPORTS = [
  { name: "mobile", width: 375, height: 812 },
  { name: "tablet", width: 768, height: 1024 },
  { name: "desktop", width: 1280, height: 800 },
  { name: "wide", width: 1920, height: 1080 },
];

for (const vp of VIEWPORTS) {
  test.describe(`Media Detail @ ${vp.name} (${vp.width}×${vp.height})`, () => {
    test("page loads without crash", async ({ page }) => {
      await page.setViewportSize(vp);
      const { jsErrors } = await collectErrors(page, async () => {
        await gotoRandomMedia(page);
        await page.waitForTimeout(1_500);
      });
      expect(jsErrors).toHaveLength(0);
    });

    test("title visible", async ({ page }) => {
      await page.setViewportSize(vp);
      test.skip(!(await gotoRandomMedia(page)), "No media");
      await expect(page.locator('[data-testid="media-title"]')).toBeVisible({ timeout: 5_000 });
    });

    test("no horizontal overflow (no scrollbar)", async ({ page }) => {
      await page.setViewportSize(vp);
      test.skip(!(await gotoRandomMedia(page)), "No media");
      const hasHScroll = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
      expect(hasHScroll).toBe(false);
    });

    test("no 5xx", async ({ page }) => {
      await page.setViewportSize(vp);
      const { serverErrors } = await collectErrors(page, async () => {
        await gotoRandomMedia(page);
        await page.waitForTimeout(1_500);
      });
      expect(serverErrors).toHaveLength(0);
    });
  });
}
