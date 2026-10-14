import { describe, expect, it } from "vitest";
import { RELOAD_RETRY_WINDOW_MS, playbackReloadRetry, reloadAtPlayhead, reloadableStatus } from "./playbackReload";

describe("reloadAtPlayhead", () => {
  it("loads the same source again, then seeks to the playhead and plays", async () => {
    const calls: string[] = [];
    const source = { uri: "https://evey.example/api/v1/media/m1/hls/master.m3u8?token=t" };
    let seekedTo = -1;
    const player = {
      replaceAsync: async (s: typeof source) => { calls.push(`replace:${s.uri}`); },
      set currentTime(v: number) { calls.push("seek"); seekedTo = v; },
      get currentTime() { return seekedTo; },
      play: () => { calls.push("play"); },
    };
    await reloadAtPlayhead(player, source, 11632.5);
    expect(calls).toEqual([`replace:${source.uri}`, "seek", "play"]);
    expect(seekedTo).toBe(11632.5);
  });

  it("leaves a failed reload to the caller", async () => {
    const player = { replaceAsync: async () => { throw new Error("released"); }, currentTime: 0, play: () => {} };
    await expect(reloadAtPlayhead(player, null, 10)).rejects.toThrow("released");
  });
});

// The message an Android phone reported for a 410 (prod, 2026-10-13), and the
// same one for a 404 (expo-video builds both from Media3's text alike).
const MEASURED_410 = "A playback exception has occurred: Source error Response code: 410";
const SAME_404 = "A playback exception has occurred: Source error Response code: 404";

describe("reloadableStatus", () => {
  it("reads the status a reload cures", () => {
    expect(reloadableStatus(MEASURED_410)).toBe(410);
    expect(reloadableStatus(SAME_404)).toBe(404);
  });

  it("is null for every other failure", () => {
    expect(reloadableStatus(undefined)).toBeNull();
    expect(reloadableStatus("A playback exception has occurred: Source error Response code: 403")).toBeNull();
    expect(reloadableStatus("A playback exception has occurred: Source error Response code: 500")).toBeNull();
    expect(reloadableStatus("Source error Response code: 4100")).toBeNull();
    expect(reloadableStatus("MediaCodecAudioRenderer error, index=2, format=Format(410, 404, ...)")).toBeNull();
  });
});

describe("playbackReloadRetry", () => {
  it("reloads on the first 410 or 404", () => {
    expect(playbackReloadRetry(MEASURED_410, null, 1_000)).toBe(true);
    expect(playbackReloadRetry(SAME_404, null, 1_000)).toBe(true);
  });

  it("shows a second one that follows a reload closely, whichever the status", () => {
    expect(playbackReloadRetry(MEASURED_410, 1_000, 1_000 + RELOAD_RETRY_WINDOW_MS - 1)).toBe(false);
    expect(playbackReloadRetry(SAME_404, 1_000, 1_000 + RELOAD_RETRY_WINDOW_MS - 1)).toBe(false);
  });

  it("reloads again once the previous reload played for a while", () => {
    expect(playbackReloadRetry(SAME_404, 1_000, 1_000 + RELOAD_RETRY_WINDOW_MS)).toBe(true);
  });

  it("leaves every other failure to the error screen", () => {
    expect(playbackReloadRetry(undefined, null, 0)).toBe(false);
    expect(playbackReloadRetry("A playback exception has occurred: Source error Response code: 403", null, 0)).toBe(false);
  });
});
