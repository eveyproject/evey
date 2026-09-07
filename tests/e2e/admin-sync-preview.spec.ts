import { test, expect } from "./fixtures/auth";

// Does the sync-preview clip actually PLAY in a browser?
//
// Everything else about this panel is covered by unit tests, and none of it
// could have caught the defect it was rebuilt for: the clip was handed to
// <video src="/api/v1/admin/media/{id}/sync-preview"> as a raw URL, that route
// is behind the admin bearer, a <video> attaches no Authorization header, and
// every render came back 401 - which the browser reports as "no video with a
// supported format or MIME type was found". A jsdom test asserting the src
// attribute would have passed throughout. Only a real browser decoding real
// bytes answers this.
//
// The source is a real 4K/1080p Matroska from the dev library: the clip is
// re-encoded server-side precisely because no browser plays HEVC in Matroska
// with AC-3, so rendering from a trivial fixture would test the wrong thing.
//
// Run against the dev stack:
//   BASE_URL=http://localhost:5173 EVEY_USER=... EVEY_PASS=... \
//     npx playwright test admin-sync-preview.spec.ts --project=chromium
// (the `evey.test` vhost Caddy serves is not in /etc/hosts on this box; the
// vite dev server proxies /api, so it is the simpler entry point).
test.use({
  launchOptions: {
    args: [
      // Same reason as hls-vod-pipeline: without this the autoplay policy
      // blocks decoding until a user gesture and currentTime never moves.
      "--autoplay-policy=no-user-gesture-required",
    ],
  },
});

test.describe("Admin media-health: the sync preview clip", () => {
  test("renders a clip the browser can decode and play", async ({ page }) => {
    // Precondition through the UI, not through page.request: the admin API is
    // behind a Bearer token that lives in localStorage, and an APIRequestContext
    // carries only cookies - so an API precondition here fails on auth and says
    // nothing about the scan. The page itself is the honest check.
    await page.goto("/admin/media-health");
    await expect(page.getByTestId("seek-index-page")).toBeVisible();

    // "All" holds every scanned file; the default view only shows what awaits
    // attention, which can legitimately be empty on a healthy library.
    await page.getByTestId("seek-index-view-all").click();
    const firstRow = page.getByTestId("seek-index-row").first();
    // No rows means no completed scan on this instance - the spec is about the
    // clip, not the scanner, so say which it is rather than failing obscurely.
    await expect(firstRow, "no scanned rows - run a scan on this instance first").toBeVisible({ timeout: 20_000 });

    await firstRow.getByTestId("seek-index-sync-preview-btn").click();
    const panel = page.getByTestId("sync-preview-panel");
    await expect(panel).toBeVisible();
    await expect(page.getByTestId("sync-preview-idle")).toBeVisible();

    // A moment every film reaches, so the render is not asked for a position
    // past the end of a short episode.
    const at = page.getByTestId("sync-preview-at");
    await at.evaluate((el: HTMLInputElement) => {
      el.value = "60";
      el.dispatchEvent(new Event("input", { bubbles: true }));
      el.dispatchEvent(new Event("change", { bubbles: true }));
    });
    await page.getByTestId("sync-preview-len").selectOption("10");
    await page.getByTestId("sync-preview-render").click();

    // ffmpeg re-encodes a 4K source down to 854px; a couple of seconds on a
    // warm box, more on a cold one.
    const video = page.getByTestId("sync-preview-video");
    await expect(video).toBeVisible({ timeout: 90_000 });
    await expect(page.getByTestId("sync-preview-error")).toHaveCount(0);

    // The bytes came through our own fetcher, not from a URL the browser
    // resolved itself - that IS the fix.
    await expect(video).toHaveAttribute("src", /^blob:/);

    // And they decode. readyState >= 2 means the browser has real frames, not
    // an element pointed at something it refused; a moving currentTime means it
    // is playing them.
    const played = await video.evaluate(async (v: HTMLVideoElement) => {
      // Race loadeddata against error AND a deadline. Waiting on loadeddata
      // alone hangs forever when the browser refuses the codec: it fires
      // "error", never "loadeddata", and the test then dies on its own timeout
      // saying nothing about why.
      const settled = await new Promise<string>((resolve) => {
        if (v.readyState >= 2) return resolve("ready");
        v.addEventListener("loadeddata", () => resolve("loadeddata"), { once: true });
        v.addEventListener("error", () => resolve("error"), { once: true });
        setTimeout(() => resolve("timeout"), 20_000);
      });
      const start = { settled, readyState: v.readyState, duration: v.duration, t0: v.currentTime };
      await v.play().catch(() => {});
      await new Promise((r) => setTimeout(r, 1500));
      return {
        ...start,
        t1: v.currentTime,
        error: v.error ? `${v.error.code}: ${v.error.message}` : null,
        // Says whether this browser build can decode what the server sent at
        // all - the difference between "our fix is wrong" and "this Chromium
        // build has no H.264".
        canPlayH264: v.canPlayType('video/mp4; codecs="avc1.42E01E, mp4a.40.2"'),
      };
    });
    // eslint-disable-next-line no-console
    console.log("sync-preview clip:", JSON.stringify(played));

    // Playwright's bundled Chromium and Firefox ship WITHOUT proprietary
    // codecs: canPlayType returns "" for H.264 and the element fails with
    // DEMUXER_ERROR_NO_SUPPORTED_STREAMS / NS_ERROR_DOM_MEDIA_METADATA_ERR.
    // That is a fact about the browser build, not about the clip - the server
    // really does return H.264 854x480 + AAC stereo (verified with ffprobe on
    // the bytes this route serves). Everything above this line is the fix and
    // it has been checked; only the decode needs a browser with codecs, so run
    // with `--project=chromium --channel=chrome` (or any build that has them)
    // to get the last assertion. Skipping is honest; failing would blame our
    // clip for the harness.
    test.skip(played.canPlayH264 === "", "this browser build has no H.264 - decode cannot be judged here");

    expect(played.error, "the browser refused the clip").toBeNull();
    expect(played.readyState, "no decoded frames - this is the 401 symptom").toBeGreaterThanOrEqual(2);
    expect(played.duration, "clip has no duration").toBeGreaterThan(0);
    expect(played.t1, "playback did not advance").toBeGreaterThan(played.t0);
  });
});
