import { test, expect } from "./fixtures/auth";

// The photo grid used to ask the server for 5000 photos in a single call. It
// never got them: any limit above maxPageSize (500) is replaced by the photo
// handler's own default of 100, so a library of thousands rendered exactly 100
// tiles - under a footer announcing the real total. Nothing errored, so nothing
// caught it.
//
// Unit tests can prove the request shape and the cursor chaining, but not that
// scrolling actually pulls the next page: that depends on the virtualizer, the
// real scroll container, and the browser. This spec is the part only a real
// browser can answer.
test.describe("Photo grid pagination", () => {
  test("scrolling past the first page loads more photos", async ({ page }) => {
    const photoRequests: string[] = [];
    page.on("request", (req) => {
      const url = req.url();
      if (url.includes("/api/v1/photos?")) photoRequests.push(url);
    });

    await page.goto("/photos", { waitUntil: "domcontentloaded" });

    // "All Photos" is the flat grid; the default view is the album list.
    // Selected by test id, not by label: the button's text follows whatever
    // language the profile is in, and the e2e suite itself changes that.
    const allPhotos = page.getByTestId("photos-view-all");
    await allPhotos.waitFor({ state: "visible", timeout: 30_000 });
    await allPhotos.click();

    const tiles = page.locator('button[aria-label], button:has(img)');
    await tiles.first().waitFor({ state: "visible", timeout: 30_000 });

    // Whatever the first page holds, no request may ask for more than the
    // server serves - that is the defect this screen shipped with.
    expect(photoRequests.length).toBeGreaterThan(0);
    for (const url of photoRequests) {
      const limit = Number(new URL(url).searchParams.get("limit") ?? "0");
      expect(limit).toBeGreaterThan(0);
      expect(limit).toBeLessThanOrEqual(500);
    }

    const firstPageRequests = photoRequests.length;
    const firstPageTiles = await tiles.count();

    // Scroll to the bottom repeatedly: the grid is virtualized, so each pass
    // renders further rows and should trigger the next page near the end.
    for (let i = 0; i < 12; i++) {
      await page.mouse.wheel(0, 20_000);
      await page.waitForTimeout(250);
      if (photoRequests.length > firstPageRequests) break;
    }

    expect(
      photoRequests.length,
      "scrolling to the end of the loaded rows must request the next page",
    ).toBeGreaterThan(firstPageRequests);

    // The follow-up request must carry the cursor, not restart at offset 0 -
    // restarting would re-serve page 1 forever.
    const followUp = photoRequests[photoRequests.length - 1];
    const params = new URL(followUp).searchParams;
    expect(params.get("cursor"), "the next page must be requested by cursor").toBeTruthy();
    expect(params.get("offset"), "a cursor page must not also carry an offset").toBeNull();

    // And the grid must actually grow: a request that loads nothing on screen
    // would be the same dead end wearing a different shape.
    await expect
      .poll(async () => tiles.count(), { timeout: 15_000 })
      .toBeGreaterThan(firstPageTiles);
  });
});
