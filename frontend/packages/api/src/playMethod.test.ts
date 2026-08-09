import { describe, expect, it } from "vitest";
import {
  effectivePlayMethod,
  isRealTranscode,
  normalizePlayMethod,
  playMethodI18nKey,
  reasonsHeadingI18nKey,
  visibleTranscodeReasons,
} from "./playMethod";

describe("normalizePlayMethod", () => {
  it("accepts the proto enum and the session snake_case alike", () => {
    expect(normalizePlayMethod("PLAY_METHOD_DIRECT_PLAY")).toBe("direct_play");
    expect(normalizePlayMethod("direct_play")).toBe("direct_play");
    expect(normalizePlayMethod("PLAY_METHOD_DIRECT_STREAM")).toBe("direct_stream");
    expect(normalizePlayMethod("direct_stream")).toBe("direct_stream");
    expect(normalizePlayMethod("PLAY_METHOD_TRANSCODE")).toBe("transcode");
    expect(normalizePlayMethod("transcode")).toBe("transcode");
  });

  it("returns the empty kind for a missing or unknown method", () => {
    expect(normalizePlayMethod(undefined)).toBe("");
    expect(normalizePlayMethod("")).toBe("");
    expect(normalizePlayMethod("PLAY_METHOD_UNSPECIFIED")).toBe("");
  });
});

describe("effectivePlayMethod", () => {
  // The bug this module exists for: E-AC-3 in Matroska on a browser that
  // decodes AC-3 in wasm. The server flags the audio codec unsupported and
  // says "transcode", but the session copies both tracks and only remuxes.
  it("calls the wasm-audio session a remux, not a transcode", () => {
    expect(
      effectivePlayMethod({
        playMethod: "PLAY_METHOD_TRANSCODE",
        videoDecision: "copy",
        audioDecision: "transcode",
        wasmAudio: true,
      }),
    ).toBe("direct_stream");
  });

  it("keeps that same session a transcode once the wasm decoder falls back", () => {
    expect(
      effectivePlayMethod({
        playMethod: "PLAY_METHOD_TRANSCODE",
        videoDecision: "copy",
        audioDecision: "transcode",
        wasmAudio: false,
      }),
    ).toBe("transcode");
  });

  // The cast receiver's audio_mode beacon writes "wasm" straight onto the
  // session, so no flag is needed for that path.
  it("treats an audio decision of wasm as a copy", () => {
    expect(
      effectivePlayMethod({ playMethod: "transcode", videoDecision: "copy", audioDecision: "wasm" }),
    ).toBe("direct_stream");
  });

  it("corrects a transcode verdict whose tracks are both copied", () => {
    expect(
      effectivePlayMethod({ playMethod: "transcode", videoDecision: "copy", audioDecision: "copy" }),
    ).toBe("direct_stream");
  });

  it("keeps a re-encoded video a transcode even when the verdict said remux", () => {
    expect(
      effectivePlayMethod({ playMethod: "direct_stream", videoDecision: "transcode", audioDecision: "copy" }),
    ).toBe("transcode");
  });

  it("never relabels direct play - nothing is touched there", () => {
    expect(effectivePlayMethod({ playMethod: "PLAY_METHOD_DIRECT_PLAY" })).toBe("direct_play");
  });

  // Cast-resurrected and music sessions carry no method: they must stay
  // unlabelled rather than gain a "Remux" badge out of thin air.
  it("stays empty when no method was declared", () => {
    expect(effectivePlayMethod({ playMethod: "", videoDecision: "copy" })).toBe("");
    expect(effectivePlayMethod({})).toBe("");
  });

  // Sessions started before the per-track fields existed have nothing to
  // correct with, so the server's verdict stands.
  it("falls back to the declared method when no track decision is known", () => {
    expect(effectivePlayMethod({ playMethod: "transcode" })).toBe("transcode");
    expect(effectivePlayMethod({ playMethod: "transcode", videoDecision: "", audioDecision: "" })).toBe("transcode");
  });

  it("decides on the tracks it does know when only one is reported", () => {
    expect(effectivePlayMethod({ playMethod: "transcode", videoDecision: "copy" })).toBe("direct_stream");
    expect(effectivePlayMethod({ playMethod: "transcode", audioDecision: "transcode" })).toBe("transcode");
  });
});

describe("isRealTranscode", () => {
  it("is false for a pure remux and true for a re-encode", () => {
    expect(isRealTranscode({ playMethod: "transcode", videoDecision: "copy", audioDecision: "copy" })).toBe(false);
    expect(isRealTranscode({ playMethod: "transcode", videoDecision: "transcode", audioDecision: "copy" })).toBe(true);
  });
});

describe("visibleTranscodeReasons", () => {
  it("drops the audio reasons the wasm path made moot", () => {
    expect(
      visibleTranscodeReasons(["audio_codec_unsupported", "audio_channels_exceeded", "container_incompatible"], true),
    ).toEqual(["container_incompatible"]);
  });

  it("keeps every reason when the wasm path is not in play", () => {
    expect(visibleTranscodeReasons(["audio_codec_unsupported", "container_incompatible"], false)).toEqual([
      "audio_codec_unsupported",
      "container_incompatible",
    ]);
  });

  it("keeps the video reasons even on the wasm path", () => {
    expect(visibleTranscodeReasons(["audio_codec_unsupported", "hdr_tonemapping_needed"], true)).toEqual([
      "hdr_tonemapping_needed",
    ]);
  });

  it("tolerates a missing list", () => {
    expect(visibleTranscodeReasons(undefined, true)).toEqual([]);
  });
});

describe("playMethodI18nKey", () => {
  it("maps each kind, and the empty kind to the empty key", () => {
    expect(playMethodI18nKey("direct_play")).toBe("player.streamInfo.methodDirectPlay");
    expect(playMethodI18nKey("direct_stream")).toBe("player.streamInfo.methodDirectStream");
    expect(playMethodI18nKey("transcode")).toBe("player.streamInfo.methodTranscode");
    expect(playMethodI18nKey("")).toBe("");
  });
});

describe("reasonsHeadingI18nKey", () => {
  it("only says transcode when a track is re-encoded", () => {
    expect(reasonsHeadingI18nKey({ playMethod: "transcode", videoDecision: "transcode" })).toBe(
      "player.streamInfo.transcodeReasons",
    );
    expect(
      reasonsHeadingI18nKey({ playMethod: "transcode", videoDecision: "copy", audioDecision: "transcode", wasmAudio: true }),
    ).toBe("player.streamInfo.noDirectPlayReasons");
  });
});
