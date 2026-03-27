import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

const USER_PAGES = [
  "/home", "/library", "/favorites", "/collections", "/playlists",
  "/music", "/photos", "/profile", "/profile/settings",
  "/profile/history", "/profile/notifications",
];

for (const path of USER_PAGES) {
  test.describe(`Page ${path}`, () => {
    test("loads without crash", async ({ page }) => {
      await goto(page, path);
      await page.waitForTimeout(2_000);
      const len = await page.evaluate(() => document.body.innerText.length);
      expect(len).toBeGreaterThan(10);
    });

    test("no JS errors", async ({ page }) => {
      const { jsErrors } = await collectErrors(page, async () => {
        await goto(page, path);
        await page.waitForTimeout(2_000);
      });
      expect(jsErrors).toHaveLength(0);
    });

    test("no 5xx", async ({ page }) => {
      const { serverErrors } = await collectErrors(page, async () => {
        await goto(page, path);
        await page.waitForTimeout(2_000);
      });
      expect(serverErrors).toHaveLength(0);
    });

    test("no untranslated keys", async ({ page }) => {
      await goto(page, path);
      await page.waitForTimeout(2_000);
      const body = await page.locator("body").textContent();
      const keys = (body!.match(/\b(?:admin|common|media|auth|errors)\.[a-z]+\.[a-zA-Z]+/g) ?? [])
        .filter(k => !k.includes("http"));
      expect(keys).toHaveLength(0);
    });

    test("no [object Object]", async ({ page }) => {
      await goto(page, path);
      await page.waitForTimeout(2_000);
      expect(await page.locator("body").textContent()).not.toContain("[object Object]");
    });

    test("no NaN", async ({ page }) => {
      await goto(page, path);
      await page.waitForTimeout(2_000);
      expect(await page.locator("body").textContent()).not.toContain("NaN");
    });
  });
}
