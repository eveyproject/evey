import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

/**
 * Navigation matrix: from each page, click first link/card, verify destination loads.
 * Tests SPA routing doesn't break across transitions.
 */

const PAGES = [
  "/home", "/library", "/favorites", "/collections", "/playlists",
  "/music", "/photos", "/profile", "/profile/settings",
  "/admin", "/admin/libraries", "/admin/users", "/admin/feeds",
  "/admin/plugins", "/admin/metadata", "/admin/webhooks",
];

// For each page, navigate there, click first internal link, verify no crash
for (const from of PAGES) {
  test(`${from} → first link → no crash`, async ({ page }) => {
    await goto(page, from);
    await page.waitForTimeout(1_500);
    const link = page.locator("a[href]:visible").first();
    if (await link.isVisible({ timeout: 1_000 }).catch(() => false)) {
      const { jsErrors } = await collectErrors(page, async () => {
        try { await link.click({ timeout: 2_000 }); } catch {}
        await page.waitForTimeout(1_000);
      });
      expect(jsErrors).toHaveLength(0);
    }
  });
}

// From each page, go back, verify no crash
for (const from of PAGES) {
  test(`${from} → link → back → no crash`, async ({ page }) => {
    await goto(page, from);
    await page.waitForTimeout(1_000);
    const link = page.locator("a[href]:visible").first();
    if (await link.isVisible({ timeout: 1_000 }).catch(() => false)) {
      try { await link.click({ timeout: 2_000 }); } catch {}
      await page.waitForTimeout(500);
      const { jsErrors } = await collectErrors(page, async () => {
        try { await page.goBack({ timeout: 2_000 }); } catch {}
        await page.waitForTimeout(500);
      });
      expect(jsErrors).toHaveLength(0);
    }
  });
}

// Double navigation: from → link → link → verify not blank
for (const from of PAGES) {
  test(`${from} → link → link → not blank`, async ({ page }) => {
    await goto(page, from);
    await page.waitForTimeout(1_000);
    for (let i = 0; i < 2; i++) {
      const link = page.locator("a[href]:visible").first();
      if (await link.isVisible({ timeout: 1_000 }).catch(() => false)) {
        try { await link.click({ timeout: 2_000 }); } catch {}
        await page.waitForTimeout(800);
      }
    }
    const len = await page.evaluate(() => document.body.innerText.length);
    expect(len).toBeGreaterThan(5);
  });
}

// Rapid navigation: click 5 links in quick succession
for (const from of PAGES.slice(0, 8)) {
  test(`${from} → rapid 5 clicks → no crash`, async ({ page }) => {
    await goto(page, from);
    await page.waitForTimeout(500);
    const { jsErrors } = await collectErrors(page, async () => {
      for (let i = 0; i < 5; i++) {
        const links = page.locator("a[href]:visible");
        const count = await links.count();
        if (count > 0) {
          const idx = Math.min(i, count - 1);
          try { await links.nth(idx).click({ timeout: 1_000 }); } catch {}
          await page.waitForTimeout(300);
        }
      }
    });
    expect(jsErrors).toHaveLength(0);
  });
}
