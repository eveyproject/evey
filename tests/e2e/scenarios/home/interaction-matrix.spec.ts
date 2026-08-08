import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

/**
 * Interaction matrix: on each page, click every visible button, verify no crash.
 * This catches broken onClick handlers, missing state, and unhandled errors.
 */

const PAGES = [
  "/home", "/library", "/favorites", "/collections", "/playlists",
  "/profile", "/profile/settings", "/profile/history",
  "/admin", "/admin/libraries", "/admin/users", "/admin/feeds",
  "/admin/plugins", "/admin/metadata", "/admin/webhooks",
  "/admin/audit", "/admin/activity", "/admin/sessions",
  "/admin/analytics", "/admin/system-health", "/admin/server-config",
];

for (const path of PAGES) {
  test(`${path} - click button[0] no crash`, async ({ page }) => {
    await goto(page, path);
    await page.waitForTimeout(1_500);
    const btn = page.locator("button:visible").first();
    if (await btn.isVisible({ timeout: 1_000 }).catch(() => false)) {
      const { jsErrors } = await collectErrors(page, async () => {
        try { await btn.click({ timeout: 1_000 }); } catch {}
        await page.waitForTimeout(500);
      });
      expect(jsErrors).toHaveLength(0);
    }
  });

  test(`${path} - click button[1] no crash`, async ({ page }) => {
    await goto(page, path);
    await page.waitForTimeout(1_500);
    const btn = page.locator("button:visible").nth(1);
    if (await btn.isVisible({ timeout: 1_000 }).catch(() => false)) {
      const { jsErrors } = await collectErrors(page, async () => {
        try { await btn.click({ timeout: 1_000 }); } catch {}
        await page.waitForTimeout(500);
      });
      expect(jsErrors).toHaveLength(0);
    }
  });

  test(`${path} - click button[2] no crash`, async ({ page }) => {
    await goto(page, path);
    await page.waitForTimeout(1_500);
    const btn = page.locator("button:visible").nth(2);
    if (await btn.isVisible({ timeout: 1_000 }).catch(() => false)) {
      const { jsErrors } = await collectErrors(page, async () => {
        try { await btn.click({ timeout: 1_000 }); } catch {}
        await page.waitForTimeout(500);
      });
      expect(jsErrors).toHaveLength(0);
    }
  });
}

// Select/dropdown interactions
for (const path of PAGES) {
  test(`${path} - interact with first select no crash`, async ({ page }) => {
    await goto(page, path);
    await page.waitForTimeout(1_500);
    const sel = page.locator("select:visible").first();
    if (await sel.isVisible({ timeout: 1_000 }).catch(() => false)) {
      // Whatever this select turns out to be, picking an option can SAVE it.
      // On /profile/settings the first select is the interface language, and
      // index 1 is German: this test used to leave the developer's own profile
      // in German for the rest of the campaign, which then made a large share
      // of the suite fail on English and French text matchers and read as a
      // regression. The test only ever wanted "interacting with a select does
      // not crash the page", so it restores what it found.
      const before = await sel.inputValue();
      const { jsErrors } = await collectErrors(page, async () => {
        const options = await sel.locator("option").count();
        if (options > 1) {
          await sel.selectOption({ index: 1 });
          await page.waitForTimeout(500);
        }
      });
      const after = await sel.inputValue().catch(() => before);
      if (after !== before) {
        await sel.selectOption(before).catch(() => {});
        await page.waitForTimeout(500);
      }
      expect(jsErrors).toHaveLength(0);
    }
  });
}

// Input interactions
for (const path of PAGES) {
  test(`${path} - type in first input no crash`, async ({ page }) => {
    await goto(page, path);
    await page.waitForTimeout(1_500);
    const input = page.locator("input:visible:not([type='hidden']):not([type='checkbox']):not([type='radio'])").first();
    if (await input.isVisible({ timeout: 1_000 }).catch(() => false)) {
      const { jsErrors } = await collectErrors(page, async () => {
        await input.fill("e2e-test");
        await page.waitForTimeout(300);
        await input.fill("");
        await page.waitForTimeout(200);
      });
      expect(jsErrors).toHaveLength(0);
    }
  });
}
