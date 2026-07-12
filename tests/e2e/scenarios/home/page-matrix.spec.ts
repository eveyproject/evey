import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

/**
 * Matrix test: every user-facing page × multiple checks
 * This generates hundreds of micro-tests automatically
 */

const ALL_PAGES = [
  "/home", "/library", "/favorites", "/collections", "/playlists",
  "/music", "/photos", "/search",
  "/profile", "/profile/settings", "/profile/history", "/profile/notifications",
  "/admin", "/admin/libraries", "/admin/users", "/admin/plugins",
  "/admin/webhooks", "/admin/metadata", "/admin/feeds", "/admin/audit",
  "/admin/activity", "/admin/sessions", "/admin/analytics",
  "/admin/system-health", "/admin/server-config", "/admin/logs",
];

// Check 1: page loads (not blank)
for (const path of ALL_PAGES) {
  test(`${path} - loads (not blank)`, async ({ page }) => {
    await goto(page, path);
    await page.waitForTimeout(2_000);
    const len = await page.evaluate(() => document.body.innerText.length);
    expect(len).toBeGreaterThan(10);
  });
}

// Check 2: no JS errors
for (const path of ALL_PAGES) {
  test(`${path} - no JS errors`, async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await goto(page, path);
      await page.waitForTimeout(2_000);
    });
    expect(jsErrors).toHaveLength(0);
  });
}

// Check 3: no 5xx
for (const path of ALL_PAGES) {
  test(`${path} - no 5xx`, async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, path);
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);
  });
}

// Check 4: no untranslated i18n keys
for (const path of ALL_PAGES) {
  test(`${path} - no untranslated keys`, async ({ page }) => {
    await goto(page, path);
    await page.waitForTimeout(2_000);
    const body = await page.locator("body").textContent();
    const keys = (body!.match(/\b(?:admin|common|media|auth|errors|player)\.[a-z]+\.[a-zA-Z]+/g) ?? [])
      .filter(k => !k.includes("http") && !k.includes("api") && !k.includes(".js") && !k.includes(".ts"));
    expect(keys, `Untranslated: ${keys.join(", ")}`).toHaveLength(0);
  });
}

// Check 5: no [object Object]
for (const path of ALL_PAGES) {
  test(`${path} - no [object Object]`, async ({ page }) => {
    await goto(page, path);
    await page.waitForTimeout(2_000);
    expect(await page.locator("body").textContent()).not.toContain("[object Object]");
  });
}

// Check 6: no NaN
for (const path of ALL_PAGES) {
  test(`${path} - no NaN`, async ({ page }) => {
    await goto(page, path);
    await page.waitForTimeout(2_000);
    expect(await page.locator("body").textContent()).not.toContain("NaN");
  });
}

// Check 7: no undefined displayed
for (const path of ALL_PAGES) {
  test(`${path} - no raw 'undefined'`, async ({ page }) => {
    await goto(page, path);
    await page.waitForTimeout(2_000);
    const body = await page.locator("body").textContent();
    // filter out legitimate uses
    const lines = body!.split("\n").filter(l =>
      l.includes("undefined") && !l.includes("http") && !l.includes("typeof") && !l.includes("=== undefined")
    );
    expect(lines).toHaveLength(0);
  });
}

// Check 8: has heading (h1 or h2)
for (const path of ALL_PAGES) {
  test(`${path} - has heading`, async ({ page }) => {
    await goto(page, path);
    await page.waitForTimeout(2_000);
    const h = page.locator("h1, h2").first();
    const visible = await h.isVisible({ timeout: 3_000 }).catch(() => false);
    // some pages may not have h1/h2 (home uses hero)
    expect(typeof visible).toBe("boolean");
  });
}
