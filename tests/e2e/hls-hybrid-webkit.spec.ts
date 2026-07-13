import { test, expect } from "@playwright/test";

// The HYBRID on-demand rung under WebKit (Safari's engine).
//
// A copy-eligible video is packaged verbatim by mkvgo (od_* URLs, zero ffmpeg)
// while its non-browser audio (AC-3) is re-encoded into a separate HLS rendition
// advertised as #EXT-X-MEDIA. That master shape is now the default for every
// client, and Safari is the strictest consumer of EXT-X-MEDIA there is
// (GROUP-ID / CODECS / DEFAULT attributes) - yet it had never been exercised.
//
// CAVEAT, on purpose: Playwright's WebKit on Linux is NOT Safari. It has no
// Apple native-HLS stack (so the player falls back to hls.js/MSE, as it does on
// any non-Apple engine) and decodes through GStreamer. This spec therefore
// proves the manifest + MSE path holds up in the WebKit engine; it does NOT
// substitute for a real iOS/macOS Safari run, where native HLS parses the
// master itself. Treat a pass here as necessary, not sufficient.
//
// "Dust Bunny": H.264 1920x804 + AC-3 5.1 fr - the exact shape of the title that
// broke in prod (2026-07-13).
const MEDIA_ID = "4ea8069e-289c-46f9-bf4b-e255d1530e19";

test.describe("hybrid rung (mkvgo video + audio rendition) under WebKit", () => {
  test("master carries EXT-X-MEDIA, video comes from mkvgo, audio plays and survives a seek", async ({
    page,
    browserName,
  }) => {
    test.skip(browserName !== "webkit", "WebKit-only: this is the Safari-engine guard");
    test.setTimeout(180_000);

    const requests: string[] = [];
    page.on("request", (r) => {
      const u = r.url();
      if (u.includes("/hls/") || u.includes("audio_") || u.includes("od_")) requests.push(u);
    });

    let masterBody = "";
    page.on("response", async (r) => {
      if (r.url().includes("master.m3u8") && r.status() === 200 && !masterBody) {
        masterBody = await r.text().catch(() => "");
      }
    });

    await page.goto(`/player/${MEDIA_ID}`);

    const video = page.locator("video").first();
    await expect(video).toBeVisible({ timeout: 30_000 });

    // A user gesture, then play muted: autoplay of a cross-origin source is
    // blocked otherwise and hls.js would never pull a segment.
    await page.locator("body").click({ position: { x: 400, y: 300 } }).catch(() => {});
    await video.evaluate((v: HTMLVideoElement) => {
      v.muted = true;
      void v.play().catch(() => {});
    });

    // Playback actually advances - the manifest was accepted and segments decode.
    await expect
      .poll(() => video.evaluate((v: HTMLVideoElement) => v.currentTime), {
        timeout: 90_000,
        intervals: [1000],
      })
      .toBeGreaterThan(0.5);

    // The master must advertise the audio group: this is the shape Safari is
    // strict about, and the one that was never validated.
    expect(masterBody, "master.m3u8 must be captured").not.toBe("");
    expect(masterBody, "master must advertise an EXT-X-MEDIA audio group").toContain(
      "#EXT-X-MEDIA:TYPE=AUDIO"
    );
    expect(masterBody, "the variant must be tied to the audio group").toContain('AUDIO="aud"');

    // Video served by the mkvgo packager (od_*), NOT re-encoded by ffmpeg.
    await expect
      .poll(() => requests.filter((u) => /od_(seg\d+\.m4s|init\.mp4)/.test(u)).length, {
        timeout: 60_000,
        intervals: [1000],
      })
      .toBeGreaterThan(0);

    // Audio pulled from its own rendition, as a separate track.
    await expect
      .poll(() => requests.filter((u) => /audio_\d+_(\d+\.m4s|init\.mp4)/.test(u)).length, {
        timeout: 60_000,
        intervals: [1000],
      })
      .toBeGreaterThan(0);

    // The engine really decoded audio (not just fetched it): WebKit exposes
    // decoded byte counters, so a silent-but-"playing" stream still fails here.
    const decodedAudio = await video.evaluate(
      (v: HTMLVideoElement & { webkitAudioDecodedByteCount?: number }) =>
        v.webkitAudioDecodedByteCount ?? -1
    );
    if (decodedAudio >= 0) {
      expect(decodedAudio, "WebKit must have decoded audio bytes").toBeGreaterThan(0);
    }

    // SEEK - the regression that made this rung unusable (audio session pinned to
    // t=0: a deep seek timed out, and once fixed, un-rebased fragments carried
    // tfdt=0 and dropped 30-minutes-in audio at the head of the timeline).
    const seekTarget = 600;
    await video.evaluate((v: HTMLVideoElement, t) => {
      v.currentTime = t;
    }, seekTarget);

    await expect
      .poll(() => video.evaluate((v: HTMLVideoElement) => v.currentTime), {
        timeout: 60_000,
        intervals: [1000],
      })
      .toBeGreaterThan(seekTarget - 5);

    // Playback resumes AT the seek point and keeps advancing - i.e. the audio
    // fragments landed on the timeline instead of at t=0.
    const before = await video.evaluate((v: HTMLVideoElement) => v.currentTime);
    await expect
      .poll(() => video.evaluate((v: HTMLVideoElement) => v.currentTime), {
        timeout: 60_000,
        intervals: [1000],
      })
      .toBeGreaterThan(before);

    const stalled = await video.evaluate((v: HTMLVideoElement) => v.readyState < 3);
    expect(stalled, "player must not be stalled after the seek").toBeFalsy();
  });
});
