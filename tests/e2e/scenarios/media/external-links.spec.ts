import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia } from "../shared/helpers";

test.describe("Media Detail > External links & badges", () => {
  test("IMDB badge links to imdb.com", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const imdb = page.locator("a", { hasText: "IMDb" }).first();
    if (await imdb.isVisible({ timeout: 2_000 }).catch(() => false)) {
      const href = await imdb.getAttribute("href");
      expect(href).toContain("imdb.com");
      expect(await imdb.getAttribute("target")).toBe("_blank");
    }
  });

  test("TMDB badge links to themoviedb.org", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const tmdb = page.locator("a", { hasText: "TMDB" }).first();
    if (await tmdb.isVisible({ timeout: 2_000 }).catch(() => false)) {
      const href = await tmdb.getAttribute("href");
      expect(href).toContain("themoviedb.org");
      expect(await tmdb.getAttribute("target")).toBe("_blank");
    }
  });

  test("external links have rel=noopener noreferrer", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const extLinks = page.locator('a[target="_blank"]');
    const count = await extLinks.count();
    for (let i = 0; i < count; i++) {
      const rel = await extLinks.nth(i).getAttribute("rel");
      expect(rel).toContain("noopener");
    }
  });
});
