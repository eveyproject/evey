import { describe, it, expect } from "vitest";
import {
  classifyPlayback,
  hdrLabel,
  resolutionLabel,
  channelLabel,
  videoStream,
} from "./playbackDifficulty";

const video = (over: Record<string, unknown> = {}) =>
  ({ type: "MEDIA_STREAM_TYPE_VIDEO", codec: "hevc", width: 3840, height: 2160, ...over }) as never;
const audio = (over: Record<string, unknown> = {}) =>
  ({ type: "MEDIA_STREAM_TYPE_AUDIO", codec: "ac3", channels: 6, ...over }) as never;

describe("classifyPlayback", () => {
  it("returns null without a decision (nothing to claim yet)", () => {
    expect(classifyPlayback(undefined, [video()])).toBeNull();
    expect(classifyPlayback({ playMethod: undefined }, [video()])).toBeNull();
  });

  it("direct play", () => {
    const d = classifyPlayback({ playMethod: "PLAY_METHOD_DIRECT_PLAY", transcodeReasons: [] }, [video()]);
    expect(d?.level).toBe("direct");
  });

  it("direct stream is a remux, not a transcode", () => {
    const d = classifyPlayback(
      { playMethod: "PLAY_METHOD_DIRECT_STREAM", transcodeReasons: ["container_incompatible"] },
      [video()],
    );
    expect(d?.level).toBe("remux");
  });

  it("transcode with the video copied is audio-only", () => {
    const d = classifyPlayback(
      {
        playMethod: "PLAY_METHOD_TRANSCODE",
        videoDecision: "copy",
        audioDecision: "transcode",
        transcodeReasons: ["audio_codec_unsupported"],
      },
      [video({ height: 1080 }), audio()],
    );
    expect(d?.level).toBe("audioOnly");
  });

  it("1080p video transcode is 'video', not 'heavy'", () => {
    const d = classifyPlayback(
      {
        playMethod: "PLAY_METHOD_TRANSCODE",
        videoDecision: "transcode",
        transcodeReasons: ["video_codec_unsupported"],
      },
      [video({ width: 1920, height: 1080 })],
    );
    expect(d?.level).toBe("video");
  });

  // The real file: an old DivX, MPEG-4 ASP 720x404 + MP3 in AVI. Everything is
  // re-encoded, but re-encoding 404 lines is not a demanding job - grading only
  // on "is the video re-encoded?" put this in the same bucket as 1080p.
  it("grades an old SD DivX as light, not demanding", () => {
    const d = classifyPlayback(
      {
        playMethod: "PLAY_METHOD_TRANSCODE",
        videoDecision: "transcode",
        audioDecision: "transcode",
        transcodeReasons: ["video_codec_unsupported", "audio_codec_unsupported", "container_incompatible"],
      },
      [video({ codec: "mpeg4", width: 720, height: 404 }), audio({ codec: "mp3", channels: 2 })],
    );
    expect(d?.level).toBe("videoSd");
    expect(d?.difficulty).toBe("moderate");
  });

  it("treats 576p as the top of SD and 720p as demanding", () => {
    const info = { playMethod: "PLAY_METHOD_TRANSCODE", videoDecision: "transcode", transcodeReasons: [] };
    expect(classifyPlayback(info, [video({ height: 576 })])?.difficulty).toBe("moderate");
    expect(classifyPlayback(info, [video({ height: 720 })])?.difficulty).toBe("demanding");
  });

  it("assumes HD when the row carries no dimensions", () => {
    const info = { playMethod: "PLAY_METHOD_TRANSCODE", videoDecision: "transcode", transcodeReasons: [] };
    expect(classifyPlayback(info, [video({ width: 0, height: 0 })])?.difficulty).toBe("demanding");
  });

  it("4K video transcode is heavy", () => {
    const d = classifyPlayback(
      {
        playMethod: "PLAY_METHOD_TRANSCODE",
        videoDecision: "transcode",
        transcodeReasons: ["video_codec_unsupported"],
      },
      [video({ height: 2160 })],
    );
    expect(d?.level).toBe("heavy");
  });

  it("HDR tonemapping is heavy even at 1080p", () => {
    const d = classifyPlayback(
      {
        playMethod: "PLAY_METHOD_TRANSCODE",
        videoDecision: "transcode",
        transcodeReasons: ["hdr_tonemapping_needed"],
      },
      [video({ height: 1080 })],
    );
    expect(d?.level).toBe("heavy");
  });

  it("does not call AC-3 an audio transcode when the browser decodes it in wasm", () => {
    const info = {
      playMethod: "PLAY_METHOD_TRANSCODE",
      videoDecision: "copy",
      audioDecision: "transcode",
      transcodeReasons: ["audio_codec_unsupported"],
    };
    const streams = [video({ codec: "h264", width: 1920, height: 1080 }), audio({ codec: "eac3", isDefault: true })];

    // Without the wasm path (Safari decodes AC-3 natively, so it never gets here)
    expect(classifyPlayback(info, streams, false)?.level).toBe("audioOnly");
    expect(classifyPlayback(info, streams, false)?.difficulty).toBe("moderate");

    // With it: the track is copied out-of-band, nothing is re-encoded.
    expect(classifyPlayback(info, streams, true)?.level).toBe("wasmAudio");
    expect(classifyPlayback(info, streams, true)?.difficulty).toBe("easy");
  });

  // The real response from the server for an MKV + E-AC-3 title on Chrome. Both
  // reasons come back together; neither is an encode once wasm handles the audio.
  it("handles the real MKV + E-AC-3 Chrome case as easy", () => {
    const info = {
      playMethod: "PLAY_METHOD_TRANSCODE",
      videoDecision: "copy",
      audioDecision: "transcode",
      transcodeReasons: ["audio_codec_unsupported", "container_incompatible"],
    };
    const streams = [video({ codec: "h264", width: 1920, height: 1080 }), audio({ codec: "eac3", isDefault: true })];
    const d = classifyPlayback(info, streams, true);
    expect(d?.level).toBe("wasmAudio");
    expect(d?.difficulty).toBe("easy");
  });

  it("a container mismatch alone is a repackage, never an audio transcode", () => {
    const info = {
      playMethod: "PLAY_METHOD_TRANSCODE",
      videoDecision: "copy",
      audioDecision: "copy",
      transcodeReasons: ["container_incompatible"],
    };
    const d = classifyPlayback(info, [video({ height: 1080 }), audio({ codec: "aac" })], false);
    expect(d?.level).toBe("remux");
    expect(d?.difficulty).toBe("easy");
  });

  it("keeps the wasm exemption to AC-3 only", () => {
    const info = {
      playMethod: "PLAY_METHOD_TRANSCODE",
      videoDecision: "copy",
      audioDecision: "transcode",
      transcodeReasons: ["audio_codec_unsupported"],
    };
    const streams = [video({ height: 1080 }), audio({ codec: "truehd", isDefault: true })];
    expect(classifyPlayback(info, streams, true)?.level).toBe("audioOnly");
  });

  it("still reports work the wasm path does not absorb", () => {
    const info = {
      playMethod: "PLAY_METHOD_TRANSCODE",
      videoDecision: "copy",
      audioDecision: "transcode",
      transcodeReasons: ["audio_codec_unsupported", "audio_channels_exceeded"],
    };
    const streams = [video({ height: 1080 }), audio({ codec: "eac3", isDefault: true })];
    const d = classifyPlayback(info, streams, true);
    expect(d?.level).toBe("audioOnly");
    expect(d?.reasons).toEqual(["audio_channels_exceeded"]);
  });

  it("maps every level onto a difficulty", () => {
    const at = (over: Record<string, unknown>, streams: unknown[]) =>
      classifyPlayback(over as never, streams as never, false)?.difficulty;
    expect(at({ playMethod: "PLAY_METHOD_DIRECT_PLAY" }, [video()])).toBe("easy");
    expect(at({ playMethod: "PLAY_METHOD_DIRECT_STREAM" }, [video()])).toBe("easy");
    expect(at({ playMethod: "PLAY_METHOD_TRANSCODE", videoDecision: "transcode", transcodeReasons: [] }, [video({ height: 480 })])).toBe("moderate");
    expect(at({ playMethod: "PLAY_METHOD_TRANSCODE", videoDecision: "transcode", transcodeReasons: [] }, [video({ height: 1080 })])).toBe("demanding");
    expect(at({ playMethod: "PLAY_METHOD_TRANSCODE", videoDecision: "transcode", transcodeReasons: [] }, [video({ height: 2160 })])).toBe("hard");
  });

  it("carries the server reasons through for the tooltip", () => {
    const d = classifyPlayback(
      {
        playMethod: "PLAY_METHOD_TRANSCODE",
        videoDecision: "transcode",
        transcodeReasons: ["video_codec_unsupported", "audio_channels_exceeded"],
      },
      [video()],
    );
    expect(d?.reasons).toEqual(["video_codec_unsupported", "audio_channels_exceeded"]);
  });
});

describe("hdrLabel", () => {
  it("reads PQ as HDR10 and HLG as HLG", () => {
    expect(hdrLabel(video({ colorTransfer: "smpte2084" }))).toBe("HDR10");
    expect(hdrLabel(video({ colorTransfer: "arib-std-b67" }))).toBe("HLG");
  });

  it("falls back to BT.2020 + 10-bit for rows probed before color_transfer", () => {
    expect(hdrLabel(video({ colorSpace: "bt2020nc", bitDepth: 10 }))).toBe("HDR10");
  });

  it("is null for SDR, and for BT.2020 at 8-bit", () => {
    expect(hdrLabel(video({ colorTransfer: "bt709" }))).toBeNull();
    expect(hdrLabel(video({ colorSpace: "bt2020nc", bitDepth: 8 }))).toBeNull();
    expect(hdrLabel(undefined)).toBeNull();
  });
});

describe("labels", () => {
  it("maps heights to marketing names", () => {
    expect(resolutionLabel(video({ height: 2160 }))).toBe("4K");
    expect(resolutionLabel(video({ height: 1080 }))).toBe("1080p");
    expect(resolutionLabel(video({ height: 480 }))).toBe("480p");
    expect(resolutionLabel(video({ height: 0 }))).toBeNull();
  });

  it("maps channel counts", () => {
    expect(channelLabel(8)).toBe("7.1");
    expect(channelLabel(6)).toBe("5.1");
    expect(channelLabel(2)).toBe("2.0");
    expect(channelLabel(1)).toBe("Mono");
    expect(channelLabel(0)).toBeNull();
    expect(channelLabel(3)).toBe("3ch");
  });

  it("picks the video stream out of the list", () => {
    expect(videoStream([audio(), video({ codec: "av1" })])?.codec).toBe("av1");
    expect(videoStream([audio()])).toBeUndefined();
  });
});
