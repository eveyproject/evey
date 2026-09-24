import { describe, it, expect } from "vitest";
import type { V1StreamInfo } from "@evey/api";
import { playingFromHlsLevel, playingFromVideoTrack, servedAudioFromUrl, servedVariant } from "./servedVariant";
import { buildStreamInfoModel } from "./streamInfoModel";

const COPY_PLAN = {
  playMethod: "PLAY_METHOD_TRANSCODE",
  videoDecision: "copy",
  audioDecision: "copy",
  videoCodec: "h264",
  mediaStreams: [{ type: "MEDIA_STREAM_TYPE_VIDEO", index: 0, codec: "h264", width: 1920, height: 1080, profile: "High", pixelFormat: "yuv420p" }],
} as unknown as V1StreamInfo;

const TRANSCODE_PLAN = {
  ...COPY_PLAN,
  videoDecision: "transcode",
  videoCodec: "h264",
  mediaStreams: [{ type: "MEDIA_STREAM_TYPE_VIDEO", index: 0, codec: "hevc", width: 3840, height: 2160 }],
} as unknown as V1StreamInfo;

// A copy master: the copy rung and its emergency fallback at half the peak.
const LADDER = [8_000_000, 3_000_000];

function video(model: ReturnType<typeof buildStreamInfoModel>) {
  const s = model?.sections.find((x) => x.kind === "video");
  return { decision: s?.decision, rows: Object.fromEntries((s?.rows ?? []).map((r) => [r.labelKey, r.value])) };
}

describe("servedVariant", () => {
  it("reads the copy rung as a copy and the fallback rung as a transcode", () => {
    expect(servedVariant(COPY_PLAN, { width: 1920, height: 1080, bitrate: 8_000_000 }, LADDER)?.copy).toBe(true);
    const fallback = servedVariant(COPY_PLAN, { width: 1920, height: 1080, bitrate: 3_000_000 }, LADDER);
    expect(fallback?.copy).toBe(false);
    expect(fallback?.codec).toBe("h264");
  });

  it("falls back on the source height when the engine lists no bitrates", () => {
    expect(servedVariant(COPY_PLAN, { height: 1080 })?.copy).toBe(true);
    expect(servedVariant(COPY_PLAN, { height: 720 })?.copy).toBe(false);
  });

  it("is always a transcode on a transcode plan, in the plan's output codec", () => {
    const v = servedVariant(TRANSCODE_PLAN, { width: 1280, height: 720, bitrate: 3_000_000 }, [8_000_000, 3_000_000]);
    expect(v).toMatchObject({ copy: false, height: 720, codec: "h264" });
  });

  it("says nothing on direct play or before a variant is known", () => {
    expect(servedVariant({ ...COPY_PLAN, playMethod: "PLAY_METHOD_DIRECT_PLAY" } as V1StreamInfo, { height: 1080 })).toBeNull();
    expect(servedVariant(COPY_PLAN, null)).toBeNull();
    expect(servedVariant(COPY_PLAN, { height: 0 })).toBeNull();
  });

  it("reads hls.js levels and expo-video tracks alike", () => {
    expect(playingFromHlsLevel({ width: 1280, height: 720, bitrate: 3_000_000, frameRate: 25, uri: "u" })).toEqual({ width: 1280, height: 720, bitrate: 3_000_000, frameRate: 25, url: "u" });
    expect(playingFromVideoTrack({ size: { width: 1280, height: 720 }, peakBitrate: 3_000_000, averageBitrate: 2_550_000, frameRate: 25, url: "u" })).toEqual({ width: 1280, height: 720, bitrate: 3_000_000, frameRate: 25, url: "u" });
    expect(playingFromVideoTrack(null)).toBeNull();
  });
});

describe("buildStreamInfoModel follows the playing variant", () => {
  it("shows the transcoded rung ABR dropped to, then the copy again", () => {
    const down = buildStreamInfoModel(TRANSCODE_PLAN, { served: servedVariant(TRANSCODE_PLAN, { width: 1280, height: 720, bitrate: 3_000_000 }, LADDER) });
    expect(video(down).decision).toBe("transcode");
    expect(video(down).rows["player.streamInfo.resolution"]).toBe("1280×720");
    expect(video(down).rows["player.streamInfo.codec"]).toBe("H264");
    expect(video(down).rows["player.streamInfo.bitrate"]).toBe("3.0 Mbps");

    const fallback = buildStreamInfoModel(COPY_PLAN, { served: servedVariant(COPY_PLAN, { width: 1920, height: 1080, bitrate: 3_000_000 }, LADDER) });
    expect(video(fallback).decision).toBe("transcode");
    expect(video(fallback).rows["player.streamInfo.profile"]).toBeUndefined();
    const method = fallback?.sections.find((s) => s.kind === "playback")?.rows.find((r) => r.labelKey === "player.streamInfo.method");
    expect(method?.valueKey).toBe("player.streamInfo.methodTranscode");

    const top = buildStreamInfoModel(COPY_PLAN, { served: servedVariant(COPY_PLAN, { width: 1920, height: 1080, bitrate: 8_000_000 }, LADDER) });
    expect(video(top).decision).toBe("copy");
    expect(video(top).rows["player.streamInfo.profile"]).toBe("High");
  });
});

// The copy master's fallback rung muxes its own audio; the server names it on
// the rung's URL (withServedAudio, transcode_ondemand.go).
const FALLBACK_URL = "https://evey.lan/api/v1/media/m1/hls/stream_0.m3u8?token=t&vd=transcode&fp=abc&servedAudio=transcode&servedAudioCodec=aac&servedAudioKbps=128";

describe("the fallback rung's own audio", () => {
  it("is read off the variant URL", () => {
    expect(servedAudioFromUrl(FALLBACK_URL)).toEqual({ decision: "transcode", codec: "aac", bitrate: 128_000 });
    expect(servedAudioFromUrl("https://evey.lan/api/v1/media/m1/hls/od_playlist.m3u8?token=t")).toBeUndefined();
    expect(servedAudioFromUrl(null)).toBeUndefined();
  });

  it("replaces the copy rung's audio in the panel while it plays, and gives it back after", () => {
    const plan = { ...COPY_PLAN, audioDecision: "copy", mediaStreams: [
      ...(COPY_PLAN.mediaStreams ?? []),
      { type: "MEDIA_STREAM_TYPE_AUDIO", index: 1, codec: "eac3", channels: 6, language: "fre", sampleRate: 48000 },
    ] } as unknown as V1StreamInfo;
    const audio = (m: ReturnType<typeof buildStreamInfoModel>) => {
      const s = m?.sections.find((x) => x.kind === "audio");
      return { decision: s?.decision, rows: Object.fromEntries((s?.rows ?? []).map((r) => [r.labelKey, r.value])) };
    };

    const onFallback = buildStreamInfoModel(plan, { audio: { absolute: 1 }, served: servedVariant(plan, { width: 1920, height: 1080, bitrate: 3_000_000, url: FALLBACK_URL }, LADDER) });
    expect(audio(onFallback).decision).toBe("transcode");
    expect(audio(onFallback).rows["player.streamInfo.codec"]).toBe("AAC");
    expect(audio(onFallback).rows["player.streamInfo.bitrate"]).toBe("128 Kbps");
    expect(audio(onFallback).rows["player.streamInfo.channels"]).toBeUndefined();
    expect(audio(onFallback).rows["player.streamInfo.language"]).toBe("fre");

    const onCopy = buildStreamInfoModel(plan, { audio: { absolute: 1 }, served: servedVariant(plan, { width: 1920, height: 1080, bitrate: 8_000_000, url: "od_playlist.m3u8?token=t" }, LADDER) });
    expect(audio(onCopy).decision).toBe("copy");
    expect(audio(onCopy).rows["player.streamInfo.codec"]).toBe("EAC3");
    expect(audio(onCopy).rows["player.streamInfo.channels"]).toBeDefined();
  });
});
