import { describe, it, expect } from "vitest";
import {
  classifyPlaybackFacts,
  hdrLabel,
  resolutionLabel,
  channelLabel,
  videoStream,
  isHeavyToStream,
} from "./playbackDifficulty";

const video = (over: Record<string, unknown> = {}) =>
  ({ type: "MEDIA_STREAM_TYPE_VIDEO", codec: "hevc", width: 3840, height: 2160, ...over }) as never;
const audio = (over: Record<string, unknown> = {}) =>
  ({ type: "MEDIA_STREAM_TYPE_AUDIO", codec: "ac3", channels: 6, ...over }) as never;

describe("classifyPlaybackFacts", () => {
  it("returns null without a decision (nothing to claim yet)", () => {
    expect(classifyPlaybackFacts(undefined)).toBeNull();
    expect(classifyPlaybackFacts({ playMethod: undefined })).toBeNull();
  });

  it("direct play", () => {
    expect(
      classifyPlaybackFacts({ playMethod: "PLAY_METHOD_DIRECT_PLAY", transcodeReasons: [] })?.level,
    ).toBe("direct");
  });

  it("direct stream is a remux, not a transcode", () => {
    const d = classifyPlaybackFacts({
      playMethod: "PLAY_METHOD_DIRECT_STREAM",
      transcodeReasons: ["container_incompatible"],
    });
    expect(d?.level).toBe("remux");
    expect(d?.difficulty).toBe("easy");
  });

  it("transcode with the video copied is audio-only", () => {
    const d = classifyPlaybackFacts({
      playMethod: "PLAY_METHOD_TRANSCODE",
      videoDecision: "copy",
      audioDecision: "transcode",
      transcodeReasons: ["audio_codec_unsupported"],
      videoHeight: 1080,
      audioCodec: "dts",
    });
    expect(d?.level).toBe("audioOnly");
    expect(d?.difficulty).toBe("moderate");
  });

  // The real file behind the complaint: an old DivX, MPEG-4 ASP 720x404 + MP3
  // in AVI. Everything is re-encoded, but re-encoding 404 lines is not a
  // demanding job - grading only on "is the video re-encoded?" put this in the
  // same bucket as 1080p.
  it("grades an old SD DivX as light, not demanding", () => {
    const d = classifyPlaybackFacts({
      playMethod: "PLAY_METHOD_TRANSCODE",
      videoDecision: "transcode",
      audioDecision: "transcode",
      transcodeReasons: ["video_codec_unsupported", "audio_codec_unsupported", "container_incompatible"],
      videoHeight: 404,
      audioCodec: "mp3",
    });
    expect(d?.level).toBe("videoSd");
    expect(d?.difficulty).toBe("moderate");
  });

  it("treats 576p as the top of SD and 720p as demanding", () => {
    const base = { playMethod: "PLAY_METHOD_TRANSCODE", videoDecision: "transcode", transcodeReasons: [] };
    expect(classifyPlaybackFacts({ ...base, videoHeight: 576 })?.difficulty).toBe("moderate");
    expect(classifyPlaybackFacts({ ...base, videoHeight: 720 })?.difficulty).toBe("demanding");
  });

  it("assumes HD when the row carries no dimensions", () => {
    expect(
      classifyPlaybackFacts({
        playMethod: "PLAY_METHOD_TRANSCODE",
        videoDecision: "transcode",
        transcodeReasons: [],
        videoHeight: 0,
      })?.difficulty,
    ).toBe("demanding");
  });

  it("1440p and above is heavy", () => {
    const base = { playMethod: "PLAY_METHOD_TRANSCODE", videoDecision: "transcode", transcodeReasons: [] };
    expect(classifyPlaybackFacts({ ...base, videoHeight: 1080 })?.difficulty).toBe("demanding");
    expect(classifyPlaybackFacts({ ...base, videoHeight: 1440 })?.difficulty).toBe("hard");
    expect(classifyPlaybackFacts({ ...base, videoHeight: 2160 })?.level).toBe("heavy");
  });

  it("HDR tonemapping is heavy even at 1080p", () => {
    const d = classifyPlaybackFacts({
      playMethod: "PLAY_METHOD_TRANSCODE",
      videoDecision: "transcode",
      transcodeReasons: ["hdr_tonemapping_needed"],
      videoHeight: 1080,
    });
    expect(d?.level).toBe("heavy");
    expect(d?.difficulty).toBe("hard");
  });

  it("does not call AC-3 an audio transcode when the browser decodes it in wasm", () => {
    const info = {
      playMethod: "PLAY_METHOD_TRANSCODE",
      videoDecision: "copy",
      audioDecision: "transcode",
      transcodeReasons: ["audio_codec_unsupported"],
      videoHeight: 1080,
      audioCodec: "eac3",
    };
    // Safari decodes AC-3 natively and never takes the wasm path.
    expect(classifyPlaybackFacts(info, false)?.level).toBe("audioOnly");
    expect(classifyPlaybackFacts(info, false)?.difficulty).toBe("moderate");
    // With wasm the track is copied out-of-band: nothing is re-encoded.
    expect(classifyPlaybackFacts(info, true)?.level).toBe("wasmAudio");
    expect(classifyPlaybackFacts(info, true)?.difficulty).toBe("easy");
  });

  // The real server response for an MKV + E-AC-3 title on Chrome: BOTH reasons
  // come back together, and neither is an encode once wasm handles the audio.
  it("handles the real MKV + E-AC-3 Chrome case as easy", () => {
    const d = classifyPlaybackFacts(
      {
        playMethod: "PLAY_METHOD_TRANSCODE",
        videoDecision: "copy",
        audioDecision: "transcode",
        transcodeReasons: ["audio_codec_unsupported", "container_incompatible"],
        videoHeight: 1080,
        audioCodec: "eac3",
      },
      true,
    );
    expect(d?.level).toBe("wasmAudio");
    expect(d?.difficulty).toBe("easy");
  });

  it("keeps the wasm exemption to AC-3 only", () => {
    const d = classifyPlaybackFacts(
      {
        playMethod: "PLAY_METHOD_TRANSCODE",
        videoDecision: "copy",
        audioDecision: "transcode",
        transcodeReasons: ["audio_codec_unsupported"],
        videoHeight: 1080,
        audioCodec: "truehd",
      },
      true,
    );
    expect(d?.level).toBe("audioOnly");
  });

  it("still reports work the wasm path does not absorb", () => {
    const d = classifyPlaybackFacts(
      {
        playMethod: "PLAY_METHOD_TRANSCODE",
        videoDecision: "copy",
        audioDecision: "transcode",
        transcodeReasons: ["audio_codec_unsupported", "audio_channels_exceeded"],
        videoHeight: 1080,
        audioCodec: "eac3",
      },
      true,
    );
    expect(d?.level).toBe("audioOnly");
    expect(d?.reasons).toEqual(["audio_channels_exceeded"]);
  });

  it("a container mismatch alone is a repackage, never an audio transcode", () => {
    const d = classifyPlaybackFacts({
      playMethod: "PLAY_METHOD_TRANSCODE",
      videoDecision: "copy",
      audioDecision: "copy",
      transcodeReasons: ["container_incompatible"],
      videoHeight: 1080,
      audioCodec: "aac",
    });
    expect(d?.level).toBe("remux");
    expect(d?.difficulty).toBe("easy");
  });

  it("carries the server reasons through for the tooltip", () => {
    const d = classifyPlaybackFacts({
      playMethod: "PLAY_METHOD_TRANSCODE",
      videoDecision: "transcode",
      transcodeReasons: ["video_codec_unsupported", "audio_channels_exceeded"],
      videoHeight: 2160,
    });
    expect(d?.reasons).toEqual(["video_codec_unsupported", "audio_channels_exceeded"]);
  });

  it("treats an unknown play method as a transcode rather than claiming it is light", () => {
    const d = classifyPlaybackFacts({
      playMethod: "PLAY_METHOD_SOMETHING_NEW",
      videoDecision: "transcode",
      videoHeight: 1080,
    });
    expect(d?.difficulty).toBe("demanding");
  });
});

describe("isHeavyToStream", () => {
  it("flags only the levels worth warning about", () => {
    expect(isHeavyToStream({ level: "heavy", difficulty: "hard", reasons: [] })).toBe(true);
    expect(isHeavyToStream({ level: "video", difficulty: "demanding", reasons: [] })).toBe(true);
    expect(isHeavyToStream({ level: "audioOnly", difficulty: "moderate", reasons: [] })).toBe(false);
    expect(isHeavyToStream({ level: "direct", difficulty: "easy", reasons: [] })).toBe(false);
    expect(isHeavyToStream(undefined)).toBe(false);
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
