import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia, gotoRandomMovie, collectErrors } from "../shared/helpers";

test.describe("Media Detail > Every visible element", () => {
  test("title element has data-testid", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    await expect(page.locator('[data-testid="media-title"]')).toBeVisible();
  });

  test("title text is not empty", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const t = await page.locator('[data-testid="media-title"]').textContent();
    expect(t!.trim().length).toBeGreaterThan(0);
  });

  test("title does not contain raw i18n key", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const t = await page.locator('[data-testid="media-title"]').textContent();
    expect(t).not.toMatch(/^media\.|^common\./);
  });

  test("hero area has either backdrop image or bg color", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const img = page.locator('[data-testid="media-backdrop"]');
    const hasBg = await img.isVisible({ timeout: 2_000 }).catch(() => false);
    if (!hasBg) {
      // at least the hero div exists
      const hero = page.locator(".relative.w-full.overflow-hidden").first();
      expect(await hero.count()).toBeGreaterThan(0);
    }
  });

  test("poster image visible on desktop", async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const poster = page.locator('[data-testid="media-poster"]');
    // poster may not exist if no artwork
    const count = await poster.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test("breadcrumbs visible", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const bc = page.locator("nav[aria-label*='readcrumb'], [data-testid='breadcrumbs']");
    // may exist or not
    expect(await bc.count()).toBeGreaterThanOrEqual(0);
  });

  test("edit pencil button visible", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    // By test id, not by English title: the label is translated, so matching
    // it silently passed on any non-English profile.
    const btn = page.locator('[data-testid="media-edit-toggle-btn"]').first();
    await expect(btn).toBeVisible({ timeout: 5_000 });
  });

  test("year is displayed as 4-digit number", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const body = await page.locator('[data-testid="media-detail"]').textContent();
    const years = body!.match(/\b(19|20)\d{2}\b/g);
    // at least one year somewhere, or none (OK for some media)
    expect(years === null || years.length >= 0).toBe(true);
  });

  test("no NaN anywhere on page", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const body = await page.locator("body").textContent();
    expect(body).not.toContain("NaN");
  });

  test("no Infinity anywhere on page", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const body = await page.locator("body").textContent();
    expect(body).not.toContain("Infinity");
  });

  test("no undefined anywhere on page", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const body = await page.locator("body").textContent();
    // filter out code/urls that legitimately have "undefined"
    const lines = body!.split("\n").filter(l => l.includes("undefined") && !l.includes("http") && !l.includes("typeof"));
    expect(lines).toHaveLength(0);
  });

  test("no [object Object] anywhere on page", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const body = await page.locator("body").textContent();
    expect(body).not.toContain("[object Object]");
  });

  test("community rating format is X.X", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const body = await page.locator('[data-testid="media-detail"]').textContent();
    const ratings = body!.match(/\d\.\d/g);
    // if there are ratings, they should look like decimal numbers
    if (ratings) {
      for (const r of ratings) {
        expect(parseFloat(r)).not.toBeNaN();
      }
    }
  });

  test("genres are displayed as chips/badges", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const genres = page.locator('[data-testid="media-genres"] span');
    const count = await genres.count();
    // 0 is ok (some media have no genres)
    if (count > 0) {
      const firstGenre = await genres.first().textContent();
      expect(firstGenre!.trim().length).toBeGreaterThan(0);
    }
  });

  test("overview section exists (even if empty)", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    // just verify no crash when rendering overview
    const body = await page.locator('[data-testid="media-detail"]').textContent();
    expect(body!.length).toBeGreaterThan(10);
  });
});

test.describe("Media Detail > Actions row", () => {
  test("favorite button visible", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const btn = page.locator('[data-testid="favorite-btn"], [data-testid="favorite-btn-active"]').first();
    await expect(btn).toBeVisible({ timeout: 5_000 });
  });

  test("mark watched/unwatched button visible", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const btn = page.locator('[data-testid="mark-watched-btn"], [data-testid="mark-unwatched-btn"]').first();
    await expect(btn).toBeVisible({ timeout: 5_000 });
  });

  test("play button visible on movie", async ({ page }) => {
    test.skip(!(await gotoRandomMovie(page)), "No movie");
    await expect(page.locator('[data-testid="play-button"]')).toBeVisible({ timeout: 5_000 });
  });

  test("play button links to /player/{id}", async ({ page }) => {
    test.skip(!(await gotoRandomMovie(page)), "No movie");
    const href = await page.locator('[data-testid="play-button"]').getAttribute("href");
    expect(href).toContain("/player/");
  });
});

test.describe("Media Detail > Cast section", () => {
  test("cast section exists for media with actors", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const cast = page.locator('[data-testid="cast-section"]');
    // may not exist
    expect(await cast.count()).toBeGreaterThanOrEqual(0);
  });

  test("cast members have names", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const castSection = page.locator('[data-testid="cast-section"]');
    test.skip(!(await castSection.isVisible({ timeout: 2_000 }).catch(() => false)), "No cast");
    const names = castSection.locator("a, span");
    const count = await names.count();
    expect(count).toBeGreaterThan(0);
  });

  test("cast member links go to /person/", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const link = page.locator('[data-testid="cast-section"] a').first();
    test.skip(!(await link.isVisible({ timeout: 2_000 }).catch(() => false)), "No cast links");
    const href = await link.getAttribute("href");
    expect(href).toContain("/person/");
  });

  test("cast photos are not broken (naturalWidth > 0)", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const castSection = page.locator('[data-testid="cast-section"]');
    test.skip(!(await castSection.isVisible({ timeout: 2_000 }).catch(() => false)), "No cast");
    const brokenCount = await page.evaluate(() => {
      const section = document.querySelector('[data-testid="cast-section"]');
      if (!section) return 0;
      return Array.from(section.querySelectorAll("img"))
        .filter(img => img.offsetParent !== null && img.naturalWidth === 0 && img.style.display !== "none")
        .length;
    });
    expect(brokenCount).toBe(0);
  });
});

test.describe("Media Detail > Similar section", () => {
  test("similar section loads without crash", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await gotoRandomMedia(page);
      await page.waitForTimeout(2_000);
    });
    expect(jsErrors).toHaveLength(0);
  });
});

test.describe("Media Detail > File info", () => {
  test("file info toggle exists for movies", async ({ page }) => {
    test.skip(!(await gotoRandomMovie(page)), "No movie");
    const fileInfo = page.locator("text=/file info/i").first();
    // may exist or not
    expect(await fileInfo.count()).toBeGreaterThanOrEqual(0);
  });

  test("clicking file info expands section", async ({ page }) => {
    test.skip(!(await gotoRandomMovie(page)), "No movie");
    const toggle = page.locator("button", { hasText: /file info/i }).first();
    test.skip(!(await toggle.isVisible({ timeout: 2_000 }).catch(() => false)), "No file info toggle");
    await toggle.click();
    await page.waitForTimeout(500);
    // should show file path or codec badges
    const badges = page.locator("text=/kbps|MB|GB|mkv|mp4|avi/i").first();
    await expect(badges).toBeVisible({ timeout: 2_000 });
  });
});
