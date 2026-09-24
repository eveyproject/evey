import { describe, it, expect } from "vitest";
import type { V1StreamInfo } from "@evey/api";
import { buildStreamInfoModel, formatBitrate } from "./streamInfoModel";

const SESSION = {
  playMethod: "PLAY_METHOD_TRANSCODE",
  videoDecision: "copy",
  audioDecision: "transcode",
  container: "matroska",
  bitrate: "5000000",
  audioBitrate: "192000",
  transcodeReasons: ["audio_codec_not_supported"],
  mediaStreams: [
    { type: "MEDIA_STREAM_TYPE_VIDEO", index: 0, codec: "hevc", width: 3840, height: 2160, frameRate: 23.976023976023978, profile: "Main 10", pixelFormat: "yuv420p10le" },
    { type: "MEDIA_STREAM_TYPE_AUDIO", index: 1, codec: "truehd", channels: 8, language: "eng", bitrate: "3000000" },
    { type: "MEDIA_STREAM_TYPE_AUDIO", index: 2, codec: "ac3", channels: 6, language: "fre", bitrate: "448000", sampleRate: 48000 },
  ],
} as unknown as V1StreamInfo;

function rows(model: ReturnType<typeof buildStreamInfoModel>, kind: string) {
  return Object.fromEntries((model?.sections.find((s) => s.kind === kind)?.rows ?? []).map((r) => [r.labelKey, r.value ?? r.valueKey]));
}

describe("buildStreamInfoModel", () => {
  it("describes the picked audio track, not the file's first one", () => {
    const audio = rows(buildStreamInfoModel(SESSION, { audio: { absolute: 2 } }), "audio");
    expect(audio["player.streamInfo.codec"]).toBe("AC3");
    expect(audio["player.streamInfo.language"]).toBe("fre");
    expect(audio["player.streamInfo.sampleRate"]).toBe("48000 Hz");
    // The served rate, not the source track's.
    expect(audio["player.streamInfo.bitrate"]).toBe("192 Kbps");
  });

  it("estimates the video bitrate as total minus audio when the probe has none", () => {
    const video = rows(buildStreamInfoModel(SESSION, { audio: { absolute: 2 } }), "video");
    expect(video["player.streamInfo.bitrate"]).toBe("≈ 4.6 Mbps");
    expect(video["player.streamInfo.framerate"]).toBe("23.976 fps");
    expect(video["player.streamInfo.profile"]).toBe("Main 10");
    expect(video["player.streamInfo.pixelFormat"]).toBe("yuv420p10le");
  });

  it("names a client-side decode, and the transcode again once the relay latched AAC", () => {
    const wasm = buildStreamInfoModel({ ...SESSION, audioDecision: "wasm" }, { wasmAudioActive: true, wasmAudioLeg: "wasm" });
    expect(wasm?.sections.find((s) => s.kind === "audio")?.decision).toBe("wasm");
    const latched = buildStreamInfoModel({ ...SESSION, audioDecision: "wasm" }, { wasmAudioActive: true, wasmAudioLeg: "aac-latched" });
    expect(latched?.sections.find((s) => s.kind === "audio")?.decision).toBe("transcode");
  });

  // Seen in the web player on 2026-09-24: the server sends audioBitrate "0"
  // for a figure it does not know, and the row read a bare "-".
  it("drops a row whose figure is unknown instead of showing a bare dash", () => {
    const model = buildStreamInfoModel({ ...SESSION, audioBitrate: "0", mediaStreams: [SESSION.mediaStreams![2]!] } as V1StreamInfo, { audio: { absolute: 2 } });
    expect(rows(model, "audio")["player.streamInfo.bitrate"]).toBe("448 Kbps");
    const none = buildStreamInfoModel({ ...SESSION, audioBitrate: "0", mediaStreams: [{ ...SESSION.mediaStreams![2]!, bitrate: undefined }] } as V1StreamInfo, { audio: { absolute: 2 } });
    expect(rows(none, "audio")["player.streamInfo.bitrate"]).toBeUndefined();
  });

  it("has nothing to show without a session", () => {
    expect(buildStreamInfoModel(null)).toBeNull();
  });

  it("writes one bitrate unit everywhere", () => {
    expect(formatBitrate("4600000")).toBe("4.6 Mbps");
    expect(formatBitrate(128000)).toBe("128 Kbps");
    expect(formatBitrate("nope")).toBe("-");
  });
});
