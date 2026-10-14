import { describe, expect, it } from "vitest";
import { STALE_RETRY_WINDOW_MS, reloadAtPlayhead, staleRenditionRetry } from "./staleRendition";

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

// The message an Android phone reported for a 410 (prod, 2026-10-13).
const MEASURED_410 = "A playback exception has occurred: Source error Response code: 410";

describe("staleRenditionRetry", () => {
  it("rebuilds on the first 410", () => {
    expect(staleRenditionRetry(MEASURED_410, null, 1_000)).toBe(true);
  });

  it("shows a second 410 that follows a rebuild closely", () => {
    expect(staleRenditionRetry(MEASURED_410, 1_000, 1_000 + STALE_RETRY_WINDOW_MS - 1)).toBe(false);
  });

  it("rebuilds again once the previous rebuild played for a while", () => {
    expect(staleRenditionRetry(MEASURED_410, 1_000, 1_000 + STALE_RETRY_WINDOW_MS)).toBe(true);
  });

  it("leaves every other failure to the error screen", () => {
    expect(staleRenditionRetry(undefined, null, 0)).toBe(false);
    expect(staleRenditionRetry("A playback exception has occurred: Source error Response code: 404", null, 0)).toBe(false);
    expect(staleRenditionRetry("Source error Response code: 4100", null, 0)).toBe(false);
    expect(staleRenditionRetry("MediaCodecAudioRenderer error, index=2, format=Format(410, ...)", null, 0)).toBe(false);
  });
});
