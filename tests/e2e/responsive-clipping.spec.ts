import { test, expect } from "./fixtures/auth";
import { gotoRandomSeries } from "./scenarios/shared/helpers";

// Responsive regression guard for phone widths.
//
// The assertion is deliberately blunt: on a phone, the main content panel must
// not scroll sideways. Anything wider than the panel is content the reader has
// to go hunting for, and on a touch device there is no visible scrollbar to
// hint that it exists.
//
// Two weaker checks were tried first and both guarded nothing:
//
//  - "the DOCUMENT has no horizontal scrollbar" (what
//    scenarios/media/responsive.spec.ts asserts) stays green here, because the
//    overflow happens INSIDE <main>, not on <html>.
//  - "an element is wider than its parent and no ancestor scrolls" also stays
//    green, because <main> carries overflow-y-auto and CSS forces the other
//    axis to `auto` when one axis is not `visible` - so <main> technically
//    scrolls sideways and every element looks reachable.
//
// This version was verified by putting the original bug back (a 545px episode
// row inside a 358px column) and watching it go red.

const PHONE = { width: 390, height: 844 };

async function mainOverflow(page: import("@playwright/test").Page) {
  return page.evaluate(() => {
    const main = document.querySelector("main");
    if (!main) return { overflow: 0, worst: null as string | null };
    const overflow = main.scrollWidth - main.clientWidth;
    if (overflow <= 2) return { overflow: 0, worst: null as string | null };
    // Name the widest offender so a failure says what to fix, not just that
    // something is too wide.
    let worst: Element | null = null;
    let worstW = 0;
    for (const el of Array.from(main.querySelectorAll("*"))) {
      const cs = getComputedStyle(el);
      if (cs.position === "fixed" || cs.position === "absolute") continue;
      if (cs.transform !== "none") continue;
      const w = el.getBoundingClientRect().width;
      if (w > worstW && w > main.clientWidth) {
        worstW = w;
        worst = el;
      }
    }
    return {
      overflow,
      worst: worst
        ? `${worst.tagName}.${(worst.className || "").toString().replace(/\s+/g, " ").slice(0, 60)} (${Math.round(worstW)}px in ${main.clientWidth}px)`
        : null,
    };
  });
}

test.describe("Responsive - the main panel never scrolls sideways on a phone", () => {
  test.use({ viewport: PHONE });

  for (const path of [
    "/home",
    "/library",
    "/search",
    "/favorites",
    "/playlists",
    "/collections",
    "/admin",
    "/admin/plugins",
    "/admin/users",
    "/profile/settings",
  ]) {
    test(`${path} fits the screen`, async ({ page }) => {
      await page.goto(path, { waitUntil: "domcontentloaded" });
      // Poll rather than sleep: under a parallel run a page can still be
      // mid-layout at any fixed delay, and a half-laid-out row measures wide.
      // A real overflow never resolves, so this cannot hide one.
      await expect
        .poll(async () => (await mainOverflow(page)).overflow, { timeout: 15_000 })
        .toBe(0);
    });
  }

  // The regression that motivated this file. Episode rows carry a fixed-width
  // still plus text plus a download control, and they live on a SEASON page -
  // a series page only lists seasons, so this has to go one level deeper or it
  // guards nothing.
  test("a season's episode rows fit the screen", async ({ page }) => {
    const seriesId = await gotoRandomSeries(page);
    test.skip(!seriesId, "No series found");
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 15_000 });

    const seasonLink = page.locator('[data-testid="seasons-section"] a[href^="/media/"]').first();
    test.skip((await seasonLink.count()) === 0, "Series has no season tiles");
    // Follow the href rather than clicking: the tile has a hover zoom
    // (group-hover:scale-105), so Playwright's stability check never settles on
    // an element that starts animating the moment it is hovered.
    const href = await seasonLink.getAttribute("href");
    test.skip(!href, "Season tile has no href");
    await page.goto(href!, { waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 15_000 });

    const episodeRows = page.locator('[data-testid="download-btn"]');
    await expect.poll(() => episodeRows.count(), { timeout: 15_000 }).toBeGreaterThan(0);

    const result = await mainOverflow(page);
    expect(result.overflow, `widest offender: ${result.worst}`).toBe(0);
  });
});
