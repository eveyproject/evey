import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

/**
 * Viewport matrix: every page × every viewport → no crash, no overflow
 */

const PAGES = [
  "/home", "/library", "/favorites", "/collections", "/playlists",
  "/music", "/photos", "/profile", "/profile/settings",
  "/admin", "/admin/libraries", "/admin/users", "/admin/feeds",
  "/admin/plugins", "/admin/metadata",
];

const VIEWPORTS = [
  { name: "iPhone SE", width: 320, height: 568 },
  { name: "iPhone 13", width: 390, height: 844 },
  { name: "iPad", width: 768, height: 1024 },
  { name: "iPad landscape", width: 1024, height: 768 },
  { name: "Laptop", width: 1366, height: 768 },
  { name: "Desktop", width: 1920, height: 1080 },
  { name: "4K", width: 2560, height: 1440 },
];

for (const vp of VIEWPORTS) {
  for (const path of PAGES) {
    test(`${path} @ ${vp.name} - no crash`, async ({ page }) => {
      await page.setViewportSize(vp);
      const { jsErrors } = await collectErrors(page, async () => {
        await goto(page, path);
        await page.waitForTimeout(1_000);
      });
      expect(jsErrors).toHaveLength(0);
    });
  }
}

// Overflow check on key pages
const OVERFLOW_PAGES = ["/home", "/library", "/favorites", "/profile", "/admin"];
for (const vp of VIEWPORTS) {
  for (const path of OVERFLOW_PAGES) {
    test(`${path} @ ${vp.name} - no horizontal overflow`, async ({ page }) => {
      await page.setViewportSize(vp);
      await goto(page, path);
      await page.waitForTimeout(1_500);
      const overflow = await page.evaluate(() =>
        document.documentElement.scrollWidth > document.documentElement.clientWidth + 10
      );
      expect(overflow).toBe(false);
    });
  }
}
