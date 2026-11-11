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

  // The server serves its default for a selection naming nothing (and the
  // screens start from 0 before the streams are known, which names the video).
  it("is the default track for a picked index the file has no audio at", () => {
    expect(pickedAudioStream(info, 9)?.index).toBe(2);
    expect(pickedAudioStream(info, 0)?.index).toBe(2);
  });

  // The server's implicit pick skips a commentary track whatever the container
  // flags; the client must read its verdict against that same track.
  it("is never a default-flagged commentary track while a main track exists", () => {
    const commentary: V1StreamInfo = {
      mediaStreams: [
        { index: 1, type: "MEDIA_STREAM_TYPE_AUDIO", codec: "aac", isDefault: true, isCommentary: true },
        { index: 2, type: "MEDIA_STREAM_TYPE_AUDIO", codec: "ac3" },
      ],
    };
    expect(pickedAudioStream(commentary, undefined)?.index).toBe(2);
    expect(pickedAudioStream(commentary, 1)?.index).toBe(1);
  });
});

describe("pickedAudioStream on a file that stores its audio first", () => {
  const audioFirst: V1StreamInfo = {
    mediaStreams: [
      { index: 0, type: "MEDIA_STREAM_TYPE_AUDIO", codec: "dts" },
      { index: 1, type: "MEDIA_STREAM_TYPE_VIDEO", codec: "h264" },
      { index: 2, type: "MEDIA_STREAM_TYPE_AUDIO", codec: "aac", isDefault: true },
    ],
  };

  it("names stream 0 when it is the one picked", () => {
    expect(pickedAudioStream(audioFirst, 0)?.index).toBe(0);
  });

  it("reads the track the server judged before the file's flags", () => {
    expect(pickedAudioStream({ ...audioFirst, audioStreamIndex: 0 }, undefined)?.index).toBe(0);
    expect(pickedAudioStream(audioFirst, undefined)?.index).toBe(2);
  });
});
