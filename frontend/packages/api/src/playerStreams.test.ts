import { describe, it, expect } from "vitest";
import { playerAudioStreams, playerSubtitleStreams, subtitleBurnMode } from "./playerStreams";
import type { V1MediaStream } from "./generated/library.schemas";

const streams = [
  { type: "MEDIA_STREAM_TYPE_VIDEO", index: 0, codec: "h264" },
  { type: "MEDIA_STREAM_TYPE_AUDIO", index: 1, language: "fre", codec: "ac3", channels: 6, isDefault: true },
  { type: "MEDIA_STREAM_TYPE_SUBTITLE", index: 2, language: "fre", codec: "hdmv_pgs_subtitle", isForced: true },
  { type: "MEDIA_STREAM_TYPE_AUDIO", index: 3, language: "eng", codec: "aac", channels: 2 },
  { type: "MEDIA_STREAM_TYPE_SUBTITLE", index: 4, language: "eng", codec: "dvd_subtitle" },
] as V1MediaStream[];

describe("playerStreams", () => {
  it("lists audio in file order with the absolute and the audio-relative index", () => {
    expect(playerAudioStreams(streams).map((s) => [s.index, s.trackIndex, s.language, s.codec, s.channels])).toEqual([
      [1, 0, "fre", "ac3", 6],
      [3, 1, "eng", "aac", 2],
    ]);
  });

  it("flags only the bitmaps no player draws as a server burn-in (PGS is drawn by the player)", () => {
    const subs = playerSubtitleStreams(streams);
    expect(subs.map((s) => [s.index, s.trackIndex, s.burnIn, s.isForced])).toEqual([
      [2, 0, false, true],
      [4, 1, true, undefined],
    ]);
  });

  // The profile's burn-in choice is what the players obey: it used to be read
  // by the server alone, which only burns what the player sends, so "always"
  // changed nothing and "never" still turned a VobSub into a transcode.
  it("follows the profile's burn-in mode", () => {
    const burn = (mode: "auto" | "always" | "never") =>
      playerSubtitleStreams([...streams, { type: "MEDIA_STREAM_TYPE_SUBTITLE", index: 5, codec: "subrip" } as V1MediaStream], mode)
        .map((s) => [s.index, s.burnIn]);
    expect(burn("auto")).toEqual([[2, false], [4, true], [5, false]]);
    expect(burn("always")).toEqual([[2, true], [4, true], [5, false]]);
    expect(burn("never")).toEqual([[2, false], [4, false], [5, false]]);
  });

  it("reads the mode off the profile, auto for anything it does not know", () => {
    expect(subtitleBurnMode({ subtitle_burn_in: "Always" })).toBe("always");
    expect(subtitleBurnMode({ subtitle_burn_in: "never" })).toBe("never");
    expect(subtitleBurnMode({ subtitle_burn_in: "sometimes" })).toBe("auto");
    expect(subtitleBurnMode(undefined)).toBe("auto");
  });

  it("answers an empty list before the stream info lands", () => {
    expect(playerAudioStreams(undefined)).toEqual([]);
    expect(playerSubtitleStreams(undefined)).toEqual([]);
  });
});
