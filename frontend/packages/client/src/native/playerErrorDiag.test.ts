import { describe, it, expect } from "vitest";
import { diagnoseNativePlayerError, playMethodName } from "./playerErrorDiag";

// The message expo-video builds from an ExoPlayer PlaybackException: its own
// message (renderer, format, format_supported) then its first cause's.
const VIDEO =
  "A playback exception has occurred: MediaCodecVideoRenderer error, index=0, " +
  "format=Format(1, null, video/mp4, video/avc, avc1.640033, 35117824, und, [3840, 2160, 60.0, ColorInfo(BT709, Limited range, SDR SMPTE 170M, false, 8bit Luma, 8bit Chroma)], [-1, -1]), " +
  "format_supported=NO_EXCEEDS_CAPABILITIES Decoder init failed: c2.vendor.avc.decoder, Format(1, null, video/mp4, video/avc, avc1.640033, 35117824, und, [3840, 2160, 60.0, null], [-1, -1])";

describe("diagnoseNativePlayerError", () => {
  it("takes a video decoder failure apart", () => {
    expect(diagnoseNativePlayerError(VIDEO, "direct_play")).toEqual({
      renderer: "video",
      formatSupported: "NO_EXCEEDS_CAPABILITIES",
      decoder: "c2.vendor.avc.decoder",
      mime: "video/avc",
      codecs: "avc1.640033",
      width: 3840,
      height: 2160,
      fps: 60,
      bitrateKbps: 35117,
      playMethod: "direct_play",
      errorCode: "",
      causes: ["Decoder init failed: c2.vendor.avc.decoder, Format(1, null, video/mp4, video/avc, avc1.640033, 35117824, und, [3840, 2160, 60.0, null], [-1, -1])"],
    });
  });

  it("names an audio renderer and keeps what the format says", () => {
    const msg =
      "A playback exception has occurred: MediaCodecAudioRenderer error, index=1, " +
      "format=Format(2, null, null, audio/ac3, null, 640000, en, [-1, -1, -1.0, null], [6, 48000]), format_supported=NO_UNSUPPORTED_TYPE ";
    const d = diagnoseNativePlayerError(msg, "transcode");
    expect(d.renderer).toBe("audio");
    expect(d.formatSupported).toBe("NO_UNSUPPORTED_TYPE");
    expect(d.mime).toBe("audio/ac3");
    expect(d.codecs).toBe("");
    expect(d.width).toBe(0);
    expect(d.fps).toBe(0);
    expect(d.bitrateKbps).toBe(640);
    expect(d.causes).toEqual([]);
  });

  it("keeps a source error whole as its cause and reads an error code", () => {
    const msg = "A playback exception has occurred: Source error ERROR_CODE_IO_BAD_HTTP_STATUS Response code: 404";
    const d = diagnoseNativePlayerError(msg, "transcode");
    expect(d.renderer).toBe("");
    expect(d.errorCode).toBe("ERROR_CODE_IO_BAD_HTTP_STATUS");
    expect(d.causes).toEqual(["Source error ERROR_CODE_IO_BAD_HTTP_STATUS Response code: 404"]);
  });

  it("never throws on a message of another shape", () => {
    for (const msg of ["", "boom", "format=Format(", "MediaCodecVideoRenderer error, format=Format(1, null"]) {
      const d = diagnoseNativePlayerError(msg, "");
      expect(d.renderer === "" || d.renderer === "video").toBe(true);
      expect(Number.isFinite(d.width)).toBe(true);
    }
  });
});

describe("playMethodName", () => {
  it("names how the stream was played", () => {
    expect(playMethodName("PLAY_METHOD_DIRECT_PLAY")).toBe("direct_play");
    expect(playMethodName("PLAY_METHOD_DIRECT_STREAM")).toBe("direct_stream");
    expect(playMethodName("PLAY_METHOD_TRANSCODE")).toBe("transcode");
    expect(playMethodName(undefined)).toBe("");
    expect(playMethodName("PLAY_METHOD_TRANSCODE", true)).toBe("offline");
  });
});
