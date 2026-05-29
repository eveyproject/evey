import { test, expect } from "./fixtures/auth";

// Targeted runtime check for the new VOD HLS pipeline (refacto/hls-vod-hybrid).
// Opens the web player on a known media item, asserts that hls.js triggers
// the new /hls/v2/ URL contract (master → variant → segment + init), and that
// a programmatic seek lands segment requests around the new position.
//
// We don't try to verify <video>.currentTime > 0 — headless Chromium's
// autoplay policy makes that unreliable, and the network-trace assertions
// below are a stronger signal anyway: if hls.js fetched a segment, the
// orchestrator served it and the cache addressed it correctly.
const MEDIA_ID = process.env.PLAYER_MEDIA_ID ?? "e1733882-a851-41d5-b714-ec5e93a8595b";

test.use({
  // Bypass autoplay-blocking so hls.js doesn't get stuck waiting on a user
  // gesture before it starts fetching segments.
  launchOptions: {
    args: ["--autoplay-policy=no-user-gesture-required"],
  },
});

test("HLS VOD pipeline — master, variant, segments + seek all hit /hls/v2/", async ({ page }) => {
  test.setTimeout(180_000);

  const hlsRequests: { url: string; status?: number }[] = [];
  page.on("request", (req) => {
    const u = req.url();
    if (u.includes("/hls/")) hlsRequests.push({ url: u });
  });
  page.on("response", (res) => {
    const u = res.url();
    if (u.includes("/hls/")) {
      const e = hlsRequests.find((r) => r.url === u && r.status === undefined);
      if (e) e.status = res.status();
    }
  });

  const consoleErrors: string[] = [];
  page.on("pageerror", (e) => consoleErrors.push(`pageerror: ${e.message}`));
  page.on("console", (msg) => {
    if (msg.type() === "error") consoleErrors.push(`console.error: ${msg.text()}`);
  });

  await page.goto(`/player/${MEDIA_ID}`);

  const video = page.locator("video").first();
  await expect(video).toBeVisible({ timeout: 30_000 });

  // hls.js with startPosition: 0 only fetches segments once the <video> is
  // playing (or seeking). Headless Chromium blocks autoplay even with the
  // policy flag for cross-origin sources, so we synthesize a user gesture
  // by clicking the player overlay before flipping play().
  await page.locator("body").click({ position: { x: 400, y: 300 } }).catch(() => {});
  await video.evaluate((v: HTMLVideoElement) => {
    v.muted = true;
    void v.play().catch(() => {});
  });

  // Wait for hls.js to actually pull a numbered segment from /hls/v2/.
  // This proves: master parsed, variant parsed, segment URL constructed,
  // orchestrator spawned the rafale, ffmpeg produced the segment, cache
  // returned it. If any of those broke, no segment ever arrives.
  await expect
    .poll(
      () => hlsRequests.filter((r) => /\/hls\/v2\/stream_\d+_\d{3}\./.test(r.url)).length,
      { timeout: 90_000, intervals: [1000] }
    )
    .toBeGreaterThan(0);

  // No legacy path should ever be hit by the frontend swap.
  const legacy = hlsRequests.filter((r) => r.url.includes("/hls/") && !r.url.includes("/hls/v2/"));
  expect(legacy, `unexpected legacy /hls/ traffic: ${legacy.map((r) => r.url).join(", ")}`).toHaveLength(0);

  // Contract sanity: master + at least one variant playlist + at least one
  // numbered segment, all under /hls/v2/.
  expect(hlsRequests.some((r) => r.url.includes("/hls/v2/master.m3u8")), "master.m3u8 must be fetched").toBeTruthy();
  expect(
    hlsRequests.some((r) => /\/hls\/v2\/stream_\d+\.m3u8/.test(r.url)),
    "at least one stream_N.m3u8 must be fetched"
  ).toBeTruthy();

  // Verify the variant manifest is real VOD (PLAYLIST-TYPE:VOD + ENDLIST).
  const variantUrl = hlsRequests.find((r) => /\/hls\/v2\/stream_\d+\.m3u8/.test(r.url))!.url;
  const variantBody = await page.evaluate(async (u) => (await fetch(u, { credentials: "include" })).text(), variantUrl);
  expect(variantBody).toContain("#EXT-X-PLAYLIST-TYPE:VOD");
  expect(variantBody).toContain("#EXT-X-ENDLIST");

  // Programmatic seek to ~5 min in. After this, requests for high-index
  // segments (≥ 95) must appear — that's the cancel-on-seek path producing
  // segments around the new position.
  const seekTarget = 300;
  await video.evaluate((v: HTMLVideoElement, t: number) => {
    v.currentTime = t;
  }, seekTarget);

  await expect
    .poll(
      () =>
        hlsRequests.filter((r) => {
          const m = r.url.match(/stream_\d+_(\d{3})/);
          return m && Number(m[1]) >= 95;
        }).length,
      { timeout: 60_000, intervals: [1000] }
    )
    .toBeGreaterThan(0);

  // Any segment that ever returned anything other than 200 (or 503 retry)
  // is a hard fail. Don't fail on 503: hls.js retries them, and our handler
  // returns 503 when ffmpeg cold-starts at a far seek.
  const bad = hlsRequests.filter((r) => r.status && ![200, 503].includes(r.status));
  expect(bad, `bad HLS responses: ${bad.map((r) => `${r.status} ${r.url}`).join(", ")}`).toHaveLength(0);

  // Surface fatal console errors. hls.js logs warnings for retried fragments;
  // those are noise, not failures.
  const fatal = consoleErrors.filter(
    (e) =>
      !e.toLowerCase().includes("retrying") &&
      !e.toLowerCase().includes("fragment") &&
      !e.toLowerCase().includes("aborted")
  );
  expect(fatal, `console errors: ${fatal.join("\n")}`).toHaveLength(0);
});
