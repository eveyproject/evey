import { describe, it, expect } from "vitest";
import type { V1StreamInfo } from "@evey/api";
import { playerDecodesCopiedAudio } from "./playSession";

const AC3GO = ["ac3", "eac3"];

// Real shape of a GetStreamInfo answer: the video, two audio tracks, a subtitle.
function info(over: Partial<V1StreamInfo> = {}): V1StreamInfo {
  return {
    audioDecision: "copy",
    mediaStreams: [
      { index: 0, type: "MEDIA_STREAM_TYPE_VIDEO", codec: "hevc" },
      { index: 1, type: "MEDIA_STREAM_TYPE_AUDIO", codec: "eac3", channels: 6, isDefault: true },
      { index: 2, type: "MEDIA_STREAM_TYPE_AUDIO", codec: "aac", channels: 6 },
      { index: 3, type: "MEDIA_STREAM_TYPE_SUBTITLE", codec: "subrip" },
    ],
    ...over,
  };
}

describe("playerDecodesCopiedAudio", () => {
  it("holds for the default E-AC-3 track the server copies to an ac3go player", () => {
    expect(playerDecodesCopiedAudio(info(), undefined, AC3GO)).toBe(true);
  });

  // Other codecs keep the ceiling: the platform's own fold would clip them.
  it("does not hold for a picked AAC 5.1 track", () => {
    expect(playerDecodesCopiedAudio(info(), 2, AC3GO)).toBe(false);
  });

  it("does not hold when the server transcodes the audio anyway", () => {
    expect(playerDecodesCopiedAudio(info({ audioDecision: "transcode" }), undefined, AC3GO)).toBe(false);
  });

  it("does not hold for a player without ac3go (iOS, older builds)", () => {
    expect(playerDecodesCopiedAudio(info(), undefined, [])).toBe(false);
  });

  it("falls back to the first audio track when none is flagged default", () => {
    const streams = info().mediaStreams!.map((s) => ({ ...s, isDefault: false }));
    expect(playerDecodesCopiedAudio(info({ mediaStreams: streams }), undefined, AC3GO)).toBe(true);
  });
});
