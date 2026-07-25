import { describe, it, expect } from "vitest";
import { buildCastStreamUrl, castSafeDirectPlay, pickCastProfile } from "./castUrl";

// Twin of the web suite (apps/web .../hlsPlayerUtils.test.ts, cast section):
// a cast URL is decoded by the RECEIVER, so it must never carry the phone's
// MediaCaps (hevc/hdr on a modern phone -> copy rung a gen 1-3 Chromecast
// cannot decode) nor its containers (mkv direct-plays on ExoPlayer, not on a
// receiver).

const SRV = "http://192.168.1.10:9091";

function h264Video(height = 1080, extra: Record<string, unknown> = {}) {
  return { type: "MEDIA_STREAM_TYPE_VIDEO", codec: "h264", bitDepth: 8, height, ...extra };
}
const aacAudio = { type: "MEDIA_STREAM_TYPE_AUDIO", codec: "aac", isDefault: true };
const eac3Audio = { type: "MEDIA_STREAM_TYPE_AUDIO", codec: "eac3", isDefault: true };

describe("pickCastProfile", () => {
  it("matches every documented model, case-insensitively", () => {
    expect(pickCastProfile("Google TV Streamer")).toMatchObject({
      videoCodecs: ["h264", "hevc", "vp9", "av1"], hdr: true, maxHeight: 2160, h264MaxHeight: 2160,
    });
    expect(pickCastProfile("google tv streamer")).toMatchObject({ hdr: true, maxHeight: 2160 });

    expect(pickCastProfile("Chromecast HD")).toMatchObject({
      videoCodecs: ["h264", "hevc", "vp9", "av1"], hdr: false, maxHeight: 1080, h264MaxHeight: 1080,
    });

    expect(pickCastProfile("Chromecast Google TV")).toMatchObject({
      videoCodecs: ["h264", "hevc", "vp9"], hdr: true, maxHeight: 2160, h264MaxHeight: 2160,
    });

    expect(pickCastProfile("Chromecast Ultra")).toMatchObject({
      videoCodecs: ["h264", "hevc", "vp9"],
      audioCodecs: ["aac", "mp3", "opus", "flac"],
      hdr: true, maxHeight: 2160, h264MaxHeight: 1080,
    });

    expect(pickCastProfile("Nest Hub")).toMatchObject({ videoCodecs: ["h264", "vp9"], hdr: false, maxHeight: 720 });
    expect(pickCastProfile("Nest Hub Max")).toMatchObject({ videoCodecs: ["h264", "vp9"], hdr: false, maxHeight: 720 });

    expect(pickCastProfile("Chromecast")).toMatchObject({
      videoCodecs: ["h264"], audioCodecs: ["aac", "mp3", "opus", "flac"], hdr: false, maxHeight: 1080, h264MaxHeight: 1080,
    });
    expect(pickCastProfile("Eureka Dongle")).toMatchObject({ videoCodecs: ["h264"], hdr: false, maxHeight: 1080 });
  });

  it("falls back to the gen 1-3 floor for unknown receivers", () => {
    expect(pickCastProfile("BRAVIA 4K VH2")).toMatchObject({ videoCodecs: ["h264"], hdr: false, maxHeight: 1080 });
    expect(pickCastProfile("")).toMatchObject({ videoCodecs: ["h264"], maxHeight: 1080 });
    expect(pickCastProfile(null)).toMatchObject({ videoCodecs: ["h264"], maxHeight: 1080 });
    expect(pickCastProfile(undefined)).toMatchObject({ videoCodecs: ["h264"], maxHeight: 1080 });
  });
});

describe("castSafeDirectPlay", () => {
  const base = {
    playMethod: "PLAY_METHOD_DIRECT_PLAY",
    container: "mp4",
    mediaStreams: [h264Video(1080), aacAudio],
  };

  it("accepts mp4 h264+aac within resolution, refuses mkv even with safe codecs", () => {
    const gen3 = pickCastProfile("Chromecast");
    expect(castSafeDirectPlay(base, gen3)).toBe(true);
    expect(castSafeDirectPlay({ ...base, container: "mkv" }, gen3)).toBe(false);
    expect(castSafeDirectPlay({ ...base, container: "matroska" }, gen3)).toBe(false);
  });

  it("refuses a 4K H.264 source on gen 3 and on Ultra (H.264 plateaus at 1080p on both)", () => {
    const info4k = { ...base, mediaStreams: [h264Video(2160), aacAudio] };
    expect(castSafeDirectPlay(info4k, pickCastProfile("Chromecast"))).toBe(false);
    expect(castSafeDirectPlay(info4k, pickCastProfile("Chromecast Ultra"))).toBe(false);
  });

  it("accepts a 4K H.264 source on the Google TV Streamer and Chromecast with Google TV", () => {
    const info4k = { ...base, mediaStreams: [h264Video(2160), aacAudio] };
    expect(castSafeDirectPlay(info4k, pickCastProfile("Google TV Streamer"))).toBe(true);
    expect(castSafeDirectPlay(info4k, pickCastProfile("Chromecast Google TV"))).toBe(true);
  });

  it("refuses hevc/10-bit/HDR on an SDR profile, accepts them on an HDR profile", () => {
    const hevcHdr = {
      ...base,
      mediaStreams: [
        { type: "MEDIA_STREAM_TYPE_VIDEO", codec: "hevc", bitDepth: 10, height: 2160, colorTransfer: "smpte2084" },
        aacAudio,
      ],
    };
    expect(castSafeDirectPlay(hevcHdr, pickCastProfile("Chromecast"))).toBe(false);
    expect(castSafeDirectPlay(hevcHdr, pickCastProfile("Chromecast Ultra"))).toBe(true);
  });

  it("accepts an eac3 default track on Google TV but refuses it on Ultra/gen 3 (no guaranteed passthrough)", () => {
    const eac3 = { ...base, mediaStreams: [h264Video(1080), eac3Audio] };
    expect(castSafeDirectPlay(eac3, pickCastProfile("Chromecast Google TV"))).toBe(true);
    expect(castSafeDirectPlay(eac3, pickCastProfile("Chromecast Ultra"))).toBe(false);
    expect(castSafeDirectPlay(eac3, pickCastProfile("Chromecast"))).toBe(false);
  });
});

describe("buildCastStreamUrl", () => {
  it("keeps the raw file URL for a receiver-safe direct play", () => {
    const url = buildCastStreamUrl(SRV, "m1", {
      playMethod: "PLAY_METHOD_DIRECT_PLAY",
      container: "mp4",
      mediaStreams: [h264Video(1080), aacAudio],
    }, "tok", pickCastProfile("Chromecast"));
    expect(url).toBe(`${SRV}/api/v1/media/m1/stream?token=tok`);
  });

  it("re-negotiates a direct-play-ineligible mkv as HLS with the receiver's codecs, not the phone's", () => {
    const url = buildCastStreamUrl(SRV, "m1", {
      playMethod: "PLAY_METHOD_DIRECT_PLAY",
      container: "mkv",
      mediaStreams: [{ ...h264Video(1080), codec: "hevc" }, aacAudio],
    }, "tok", pickCastProfile("Chromecast"));
    expect(url).toContain(`${SRV}/api/v1/media/m1/hls/master.m3u8`);
    expect(url).toContain("token=tok");
    expect(url).toContain("videoCodecs=h264");
    expect(url).not.toContain("hevc");
    expect(url).toContain("containers=mp4");
    expect(url).not.toContain("mkv");
    expect(url).not.toContain("hdr=");
  });

  it("carries the receiver's maxHeight/hdr/codecs onto the HLS URL", () => {
    const url = buildCastStreamUrl(SRV, "m1", {
      playMethod: "PLAY_METHOD_TRANSCODE",
      container: "mkv",
      mediaStreams: [],
    }, "tok", pickCastProfile("Chromecast Ultra"));
    expect(url).toContain("videoCodecs=h264%2Chevc%2Cvp9");
    expect(url).toContain("hdr=1");
    expect(url).toContain("maxHeight=2160");
  });

  it("omits hdr for an SDR profile", () => {
    const url = buildCastStreamUrl(SRV, "m1", {
      playMethod: "PLAY_METHOD_TRANSCODE",
      container: "mkv",
      mediaStreams: [],
    }, "tok", pickCastProfile("Nest Hub"));
    expect(url).not.toContain("hdr=");
    expect(url).toContain("maxHeight=720");
  });

  it("percent-encodes the media id (deep-link controlled)", () => {
    const url = buildCastStreamUrl(
      SRV, "..%2Fadmin",
      { playMethod: "PLAY_METHOD_TRANSCODE", mediaStreams: [] },
      "tok", pickCastProfile(null),
    );
    expect(url).toContain(encodeURIComponent("..%2Fadmin"));
  });
});

describe("pickCastProfile - evidence-driven entries from the prod beacon", () => {
  // MiTV-AFMU0: first real model string collected by cast_receiver_model
  // (2026-07-25). a tablet brand Mi TV 4S - Dolby Audio certified, HEVC 4K HDR10.
  // Before this entry it fell to the floor and its E-AC3 default track was
  // pointlessly transcoded to AAC.
  it("gives the Mi TV its Dolby and HEVC back", () => {
    const p = pickCastProfile("MiTV-AFMU0");
    expect(p.audioCodecs).toContain("eac3");
    expect(p.videoCodecs).toContain("hevc");
    expect(p.hdr).toBe(true);
    expect(p.h264MaxHeight).toBe(1080);
  });
});
