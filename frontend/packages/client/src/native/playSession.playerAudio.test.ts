import { describe, it, expect } from "vitest";
import type { V1StreamInfo } from "@evey/api";
import { ceilingChangesVerdict, playerDecodesCopiedAudio } from "./playSession";

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

// The first verdict is asked without the output ceiling; the session asks
// again with it exactly when the master will carry it AND it folds the
// track (channels above the ceiling) - the case where the panel used to
// announce a 6ch copy of segments re-encoded to stereo.
describe("ceilingChangesVerdict", () => {
  it("holds for a 5.1 track on a stereo output", () => {
    expect(ceilingChangesVerdict(info(), undefined, 2)).toBe(true);
    expect(ceilingChangesVerdict(info(), 2, 2)).toBe(true);
  });

  it("does not hold when the track fits the ceiling, or the ceiling is unknown", () => {
    expect(ceilingChangesVerdict(info(), undefined, 6)).toBe(false);
    expect(ceilingChangesVerdict(info(), undefined, 8)).toBe(false);
    expect(ceilingChangesVerdict(info(), undefined, undefined)).toBe(false);
    expect(ceilingChangesVerdict(info(), undefined, 0)).toBe(false);
  });

  it("does not hold for a track of unknown layout - the server keeps its default", () => {
    const streams = info().mediaStreams!.map((s) => ({ ...s, channels: undefined }));
    expect(ceilingChangesVerdict(info({ mediaStreams: streams }), undefined, 2)).toBe(false);
  });
});
