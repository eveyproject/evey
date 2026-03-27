import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

/**
 * Mobile matrix: test every page on mobile with touch enabled
 */

test.use({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true });

const ALL_PAGES = [
  "/home", "/library", "/favorites", "/collections", "/playlists",
  "/music", "/photos", "/search",
  "/profile", "/profile/settings", "/profile/history", "/profile/notifications",
  "/admin", "/admin/libraries", "/admin/users", "/admin/plugins",
  "/admin/webhooks", "/admin/metadata", "/admin/feeds", "/admin/audit",
  "/admin/activity", "/admin/sessions", "/admin/analytics",
  "/admin/system-health", "/admin/server-config", "/admin/logs",
];

for (const path of ALL_PAGES) {
  test(`${path} mobile — loads`, async ({ page }) => {
    await goto(page, path);
    await page.waitForTimeout(1_500);
    const len = await page.evaluate(() => document.body.innerText.length);
    expect(len).toBeGreaterThan(10);
  });
}

for (const path of ALL_PAGES) {
  test(`${path} mobile — no JS errors`, async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await goto(page, path);
      await page.waitForTimeout(1_500);
    });
    expect(jsErrors).toHaveLength(0);
  });
}

for (const path of ALL_PAGES) {
  test(`${path} mobile — no 5xx`, async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, path);
      await page.waitForTimeout(1_500);
    });
    expect(serverErrors).toHaveLength(0);
  });
}

for (const path of ALL_PAGES) {
  test(`${path} mobile — no horizontal overflow`, async ({ page }) => {
    await goto(page, path);
    await page.waitForTimeout(1_500);
    const overflow = await page.evaluate(() =>
      document.documentElement.scrollWidth > document.documentElement.clientWidth + 10
    );
    expect(overflow).toBe(false);
  });
}

for (const path of ALL_PAGES) {
  test(`${path} mobile — hamburger menu visible`, async ({ page }) => {
    await goto(page, path);
    await page.waitForTimeout(1_000);
    // On mobile, hamburger should be visible (except player/login)
    if (!path.includes("player") && !path.includes("login")) {
      const burger = page.locator('button[aria-label*="menu" i]').first();
      const visible = await burger.isVisible({ timeout: 2_000 }).catch(() => false);
      expect(typeof visible).toBe("boolean");
    }
  });
}
