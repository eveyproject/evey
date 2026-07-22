import { describe, it, expect } from "vitest";
import { buildCastStreamUrl, castReceiverCapabilities, castSafeDirectPlay, receiverSupportsUhd } from "./castUrl";

// Twin of the web suite (apps/web .../hlsPlayerUtils.test.ts, cast section):
// a cast URL is decoded by the RECEIVER, so it must never carry the phone's
// MediaCaps (hevc/hdr on a modern phone -> copy rung a gen 1-3 Chromecast
// cannot decode) nor its containers (mkv direct-plays on ExoPlayer, not on a
// receiver).

const h264Video = { type: "MEDIA_STREAM_TYPE_VIDEO", codec: "h264", bitDepth: 8 };
const aacAudio = { type: "MEDIA_STREAM_TYPE_AUDIO", codec: "aac", isDefault: true };
const SRV = "http://192.168.1.10:9091";

// The Cast SDK reports the receiver's model name once a session is up - the
// only capability signal available (no SDK exposes codec support for the
// default receiver). Unknown models get the floor: a wrong "capable" guess is
// a black screen, a wrong "floor" guess is just a transcode.
describe("receiverSupportsUhd", () => {
  it("recognizes the HEVC/HDR-capable models", () => {
    expect(receiverSupportsUhd("Chromecast Ultra")).toBe(true);
    expect(receiverSupportsUhd("Chromecast Google TV")).toBe(true);
    expect(receiverSupportsUhd("Google TV Streamer")).toBe(true);
  });

  it("keeps gen 1-3 / Audio / unknown models on the floor", () => {
    expect(receiverSupportsUhd("Chromecast")).toBe(false);
    expect(receiverSupportsUhd("Eureka Dongle")).toBe(false);
    expect(receiverSupportsUhd("Chromecast Audio")).toBe(false);
    expect(receiverSupportsUhd("MiTV-AESP0")).toBe(false);
    expect(receiverSupportsUhd("")).toBe(false);
    expect(receiverSupportsUhd(null)).toBe(false);
    expect(receiverSupportsUhd(undefined)).toBe(false);
  });
});

describe("castReceiverCapabilities", () => {
  it("floor is H.264 SDR, aac/mp3, mp4", () => {
    const caps = castReceiverCapabilities(false);
    expect(caps.videoCodecs).toEqual(["h264"]);
    expect(caps.audioCodecs).toEqual(["aac", "mp3"]);
    expect(caps.containers).toEqual(["mp4"]);
    expect(caps.hdr).toBe(false);
  });

  it("uhd adds hevc + hdr, keeps the audio floor", () => {
    const caps = castReceiverCapabilities(true);
    expect(caps.videoCodecs).toContain("hevc");
    expect(caps.hdr).toBe(true);
    expect(caps.audioCodecs).toEqual(["aac", "mp3"]);
  });
});

describe("castSafeDirectPlay", () => {
  const safe = {
    playMethod: "PLAY_METHOD_DIRECT_PLAY",
    container: "mp4",
    mediaStreams: [h264Video, aacAudio],
  };

  it("accepts mp4 h264+aac, refuses mkv even with safe codecs", () => {
    expect(castSafeDirectPlay(safe, castReceiverCapabilities(false))).toBe(true);
    expect(castSafeDirectPlay({ ...safe, container: "mkv" }, castReceiverCapabilities(false))).toBe(false);
    expect(castSafeDirectPlay({ ...safe, container: "matroska" }, castReceiverCapabilities(false))).toBe(false);
  });

  it("refuses hevc/10-bit/HDR on the floor, accepts them with uhd", () => {
    const hevcHdr = {
      ...safe,
      mediaStreams: [
        { type: "MEDIA_STREAM_TYPE_VIDEO", codec: "hevc", bitDepth: 10, colorTransfer: "smpte2084" },
        aacAudio,
      ],
    };
    expect(castSafeDirectPlay(hevcHdr, castReceiverCapabilities(false))).toBe(false);
    expect(castSafeDirectPlay(hevcHdr, castReceiverCapabilities(true))).toBe(true);
  });

  it("refuses a Dolby default audio track in both profiles", () => {
    const eac3 = { ...safe, mediaStreams: [h264Video, { ...aacAudio, codec: "eac3" }] };
    expect(castSafeDirectPlay(eac3, castReceiverCapabilities(false))).toBe(false);
    expect(castSafeDirectPlay(eac3, castReceiverCapabilities(true))).toBe(false);
  });
});

describe("buildCastStreamUrl", () => {
  it("keeps the raw file URL for a receiver-safe direct play", () => {
    const url = buildCastStreamUrl(SRV, "m1", {
      playMethod: "PLAY_METHOD_DIRECT_PLAY",
      container: "mp4",
      mediaStreams: [h264Video, aacAudio],
    }, "tok", false);
    expect(url).toBe(`${SRV}/api/v1/media/m1/stream?token=tok`);
  });

  it("re-negotiates everything else as HLS with the receiver caps, not the phone's", () => {
    const url = buildCastStreamUrl(SRV, "m1", {
      playMethod: "PLAY_METHOD_DIRECT_PLAY",
      container: "mkv",
      mediaStreams: [{ ...h264Video, codec: "hevc" }, aacAudio],
    }, "tok", false);
    expect(url).toContain(`${SRV}/api/v1/media/m1/hls/master.m3u8`);
    expect(url).toContain("token=tok");
    expect(url).toContain("videoCodecs=h264");
    expect(url).not.toContain("hevc");
    expect(url).not.toContain("mkv");
    expect(url).not.toContain("hdr=");
  });

  it("uhd advertises hevc + hdr=1 so the server keeps the copy rung", () => {
    const url = buildCastStreamUrl(SRV, "m1", {
      playMethod: "PLAY_METHOD_TRANSCODE",
      container: "mkv",
      mediaStreams: [],
    }, "tok", true);
    expect(url).toContain("videoCodecs=h264%2Chevc");
    expect(url).toContain("hdr=1");
  });

  it("percent-encodes the media id (deep-link controlled)", () => {
    const url = buildCastStreamUrl(SRV, "..%2Fadmin", { playMethod: "PLAY_METHOD_TRANSCODE", mediaStreams: [] }, "tok", false);
    expect(url).toContain(encodeURIComponent("..%2Fadmin"));
  });
});
