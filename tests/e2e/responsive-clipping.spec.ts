import { test, expect } from "./fixtures/auth";
import { gotoRandomSeries } from "./scenarios/shared/helpers";

// Responsive regression guard.
//
// The layout fixes this pins were all found the same way: an element wider than
// its parent, inside a parent that does NOT scroll. That combination is worse
// than a horizontal scrollbar - the content is simply unreachable. The episode
// list was the worst case: a 712px row inside a 374px column, so the synopsis
// and the download button sat off-screen with no way to reach them.
//
// A plain "document doesn't scroll horizontally" assertion does NOT catch that,
// because the overflow is clipped rather than scrolled. Hence the DOM walk.

const PHONE = { width: 390, height: 844 };

/** Elements sticking out of a parent that offers no way to scroll to them. */
async function clippedElements(page: import("@playwright/test").Page): Promise<string[]> {
  return page.evaluate(() => {
    const out: string[] = [];
    for (const el of Array.from(document.querySelectorAll("body *"))) {
      const parent = el.parentElement;
      if (!parent) continue;
      const b = el.getBoundingClientRect();
      const pb = parent.getBoundingClientRect();
      // Ignore slivers: sub-40px boxes are icons and separators whose rounding
      // noise would make this flaky.
      if (b.width < 40 || pb.width < 40) continue;
      const cs = getComputedStyle(el);
      // Absolutely positioned boxes resolve against a different containing
      // block, and a hover zoom (scale-105) inflates the measured rect by 5%
      // without any real overflow. Neither proves anything here.
      if (cs.position === "fixed" || cs.position === "absolute") continue;
      if (cs.transform !== "none") continue;
      if (b.right <= pb.right + 4) continue;

      // An ancestor declaring overflow-x auto/scroll makes the content
      // reachable, full stop. Do NOT additionally require
      // scrollWidth > clientWidth: that is a snapshot of layout at one instant
      // (a carousel still measuring, a chart not yet drawn) and it made this
      // check report reachable content as clipped.
      let ancestor: HTMLElement | null = parent;
      let scrollable = false;
      while (ancestor && ancestor !== document.body) {
        const s = getComputedStyle(ancestor);
        if (s.overflowX === "auto" || s.overflowX === "scroll") {
          scrollable = true;
          break;
        }
        ancestor = ancestor.parentElement;
      }
      if (scrollable) continue;

      const cls = (el.className || "").toString().replace(/\s+/g, " ").slice(0, 60);
      out.push(`${el.tagName}.${cls} [${Math.round(b.width)}px in ${Math.round(pb.width)}px]`);
    }
    return Array.from(new Set(out));
  });
}

test.describe("Responsive - nothing clipped out of reach on a phone", () => {
  test.use({ viewport: PHONE });

  for (const path of ["/home", "/library", "/search", "/favorites", "/playlists", "/admin", "/admin/plugins"]) {
    test(`${path} keeps its content reachable`, async ({ page }) => {
      await page.goto(path, { waitUntil: "domcontentloaded" });
      // Poll rather than sleep a fixed amount. Under a parallel run the dev
      // server is slow enough that a page is still mid-layout at any constant
      // delay, and a half-laid-out row looks exactly like a clipped one. A real
      // clip never resolves, so polling cannot hide one - it only removes the
      // flake.
      await expect.poll(() => clippedElements(page), { timeout: 15_000 }).toEqual([]);
    });
  }

  // The regression that motivated this file. A season page renders the episode
  // rows, which carry a fixed-width still plus text plus a download control.
  test("a season's episode rows fit the screen", async ({ page }) => {
    const id = await gotoRandomSeries(page);
    test.skip(!id, "No series found");
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 15_000 });
    await page.waitForTimeout(1500);

    const rows = page.locator('[data-testid="media-detail"] .flex.min-w-0');
    if ((await rows.count()) === 0) test.skip(true, "Series page shows seasons, not episodes");

    await expect.poll(() => clippedElements(page), { timeout: 15_000 }).toEqual([]);
  });
});
