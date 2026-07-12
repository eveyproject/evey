import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

const PAGES = [
  "/home", "/library", "/favorites", "/collections", "/playlists",
  "/music", "/photos", "/profile", "/profile/settings",
  "/admin", "/admin/libraries", "/admin/users", "/admin/feeds",
];

const VIEWPORTS = [
  { name: "small-mobile", width: 320, height: 568 },
  { name: "mobile", width: 375, height: 812 },
  { name: "tablet-portrait", width: 768, height: 1024 },
  { name: "tablet-landscape", width: 1024, height: 768 },
  { name: "laptop", width: 1366, height: 768 },
  { name: "desktop", width: 1920, height: 1080 },
];

for (const vp of VIEWPORTS) {
  for (const path of PAGES) {
    test(`${path} @ ${vp.name} (${vp.width}×${vp.height}) - no crash`, async ({ page }) => {
      await page.setViewportSize(vp);
      const { jsErrors } = await collectErrors(page, async () => {
        await goto(page, path);
        await page.waitForTimeout(1_500);
      });
      expect(jsErrors).toHaveLength(0);
    });
  }
}

for (const vp of VIEWPORTS) {
  test(`home @ ${vp.name} - no horizontal overflow`, async ({ page }) => {
    await page.setViewportSize(vp);
    await goto(page, "/home");
    await page.waitForTimeout(2_000);
    const overflow = await page.evaluate(() =>
      document.documentElement.scrollWidth > document.documentElement.clientWidth + 5
    );
    expect(overflow).toBe(false);
  });
}
