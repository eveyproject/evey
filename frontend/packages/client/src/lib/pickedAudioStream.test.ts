import { describe, it, expect } from "vitest";
import type { V1StreamInfo } from "@evey/api";
import { pickedAudioStream } from "./pickedAudioStream";

const info: V1StreamInfo = {
  mediaStreams: [
    { index: 0, type: "MEDIA_STREAM_TYPE_VIDEO", codec: "h264" },
    { index: 1, type: "MEDIA_STREAM_TYPE_AUDIO", codec: "truehd" },
    { index: 2, type: "MEDIA_STREAM_TYPE_AUDIO", codec: "ac3", isDefault: true },
    { index: 3, type: "MEDIA_STREAM_TYPE_AUDIO", codec: "aac" },
  ],
};

describe("pickedAudioStream", () => {
  it("is the picked track when one is named", () => {
    expect(pickedAudioStream(info, 3)?.index).toBe(3);
  });

  it("is the default-flagged track when none is picked", () => {
    expect(pickedAudioStream(info, undefined)?.index).toBe(2);
    expect(pickedAudioStream(info, 0)?.index).toBe(2);
  });

  it("is the first audio track when none is flagged default", () => {
    const noDefault: V1StreamInfo = {
      mediaStreams: (info.mediaStreams ?? []).map((s) => ({ ...s, isDefault: false })),
    };
    expect(pickedAudioStream(noDefault, undefined)?.index).toBe(1);
  });

  it("is nothing for a picked index the file does not have", () => {
    expect(pickedAudioStream(info, 9)).toBeUndefined();
  });
});
