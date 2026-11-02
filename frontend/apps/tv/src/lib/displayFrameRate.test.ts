import { describe, it, expect } from "vitest";
import type { V1StreamInfo } from "@evey/api";
import { playedFrameRate } from "./displayFrameRate";

const info = (playMethod: string, frameRate?: number): V1StreamInfo =>
  ({
    playMethod,
    mediaStreams: [
      { type: "MEDIA_STREAM_TYPE_AUDIO" },
      { type: "MEDIA_STREAM_TYPE_VIDEO", frameRate },
    ],
  }) as V1StreamInfo;

describe("playedFrameRate", () => {
  it("is the source rate for a direct play, whatever it is", () => {
    expect(playedFrameRate(info("PLAY_METHOD_DIRECT_PLAY", 25))).toBe(25);
    expect(playedFrameRate(info("PLAY_METHOD_DIRECT_PLAY", 59.94))).toBe(59.94);
  });

  it("trusts a transcode only at a rate no rung changes", () => {
    expect(playedFrameRate(info("PLAY_METHOD_TRANSCODE", 23.976))).toBe(23.976);
    expect(playedFrameRate(info("PLAY_METHOD_TRANSCODE", 60))).toBeNull();
  });

  it("is null without stream info or a video rate", () => {
    expect(playedFrameRate(null)).toBeNull();
    expect(playedFrameRate(info("PLAY_METHOD_DIRECT_PLAY"))).toBeNull();
  });
});
