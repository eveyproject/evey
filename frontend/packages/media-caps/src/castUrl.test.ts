import { describe, it, expect } from "vitest";
import { buildCastStreamUrl, castProfileFor, castStreamInfoParams, FLOOR_PROFILE, h264HeightFromLevel, pickCastProfile, profileFromReceiverCaps } from "./castUrl";

// Shared by the mobile and web senders: a cast URL is decoded by the
// RECEIVER, so it must never carry the sender's MediaCaps (hevc/hdr on a
// modern phone or desktop -> copy rung a gen 1-3 Chromecast cannot decode)
// nor its containers (mkv direct-plays on ExoPlayer, not on a receiver).

const SRV = "http://192.168.1.10:9091";


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

// The receiver's direct play is the SERVER's verdict (GetStreamInfo with the
// receiver's profile): this side only declares the receiver. Every rule the
// old client-side gate held - mp4 only, the codec list, the H.264 ceiling
// below the general one, HDR, the default audio track - is what these params
// carry, and the server applies them like for any other client.
describe("castStreamInfoParams", () => {
  it("declares the receiver, mp4 only, with its own H.264 ceiling", () => {
    expect(castStreamInfoParams(pickCastProfile("Chromecast Ultra"))).toEqual({
      supportedCodecs: ["h264", "hevc", "vp9"],
      supportedAudioCodecs: ["aac", "mp3", "opus", "flac"],
      supportedContainers: ["mp4"],
      supportsHdr: true,
      maxHeight: 2160,
      maxH264Height: 1080,
    });
    expect(castStreamInfoParams(pickCastProfile("Chromecast"))).toMatchObject({
      supportedCodecs: ["h264"], supportsHdr: false, maxHeight: 1080, maxH264Height: 1080,
    });
  });

  // A receiver that answered its output ceiling has it on the master URL
  // (buildCastStreamUrl); the verdict is asked with the same ceiling, so the
  // two never disagree on the channels the segments carry.
  it("declares the receiver's output ceiling exactly when the URL carries it", () => {
    const stereo = { ...pickCastProfile("Chromecast Ultra"), maxAudioChannels: 2 };
    expect(castStreamInfoParams(stereo)).toMatchObject({ maxAudioChannels: 2 });
    expect(new URL(buildCastStreamUrl(SRV, "m1", null, "tok", stereo)).searchParams.get("maxAudioChannels")).toBe("2");

    const unknown = pickCastProfile("Chromecast Ultra");
    expect(castStreamInfoParams(unknown)).not.toHaveProperty("maxAudioChannels");
    expect(new URL(buildCastStreamUrl(SRV, "m1", null, "tok", unknown)).searchParams.has("maxAudioChannels")).toBe(false);
  });
});

describe("buildCastStreamUrl", () => {
  it("keeps the raw file URL when the server's receiver verdict is direct play", () => {
    const url = buildCastStreamUrl(SRV, "m1", { playMethod: "PLAY_METHOD_DIRECT_PLAY" }, "tok", pickCastProfile("Chromecast"));
    expect(url).toBe(`${SRV}/api/v1/media/m1/stream?token=tok`);
  });

  it("plays it safe on the HLS master when no receiver verdict is in hand", () => {
    const url = buildCastStreamUrl(SRV, "m1", null, "tok", pickCastProfile("Chromecast"));
    expect(url).toContain(`${SRV}/api/v1/media/m1/hls/master.m3u8`);
  });

  it("serves a direct_stream / transcode verdict as HLS with the receiver's codecs, not the phone's", () => {
    const url = buildCastStreamUrl(SRV, "m1", { playMethod: "PLAY_METHOD_DIRECT_STREAM" }, "tok", pickCastProfile("Chromecast"));
    expect(url).toContain(`${SRV}/api/v1/media/m1/hls/master.m3u8`);
    expect(url).toContain("token=tok");
    expect(url).toContain("videoCodecs=h264");
    expect(url).not.toContain("hevc");
    expect(url).toContain("containers=mp4");
    expect(url).not.toContain("mkv");
    expect(url).not.toContain("hdr=");
  });

  it("carries the receiver's maxHeight/H.264 ceiling/hdr/codecs onto the HLS URL", () => {
    const url = buildCastStreamUrl(SRV, "m1", { playMethod: "PLAY_METHOD_TRANSCODE" }, "tok", pickCastProfile("Chromecast Ultra"));
    expect(url).toContain("videoCodecs=h264%2Chevc%2Cvp9");
    expect(url).toContain("hdr=1");
    expect(url).toContain("maxHeight=2160");
    expect(url).toContain("maxH264Height=1080");
  });

  it("omits hdr for an SDR profile", () => {
    const url = buildCastStreamUrl(SRV, "m1", { playMethod: "PLAY_METHOD_TRANSCODE" }, "tok", pickCastProfile("Nest Hub"));
    expect(url).not.toContain("hdr=");
    expect(url).toContain("maxHeight=720");
  });

  it("percent-encodes the media id (deep-link controlled)", () => {
    const url = buildCastStreamUrl(
      SRV, "..%2Fadmin",
      { playMethod: "PLAY_METHOD_TRANSCODE" },
      "tok", pickCastProfile(null),
    );
    expect(url).toContain(encodeURIComponent("..%2Fadmin"));
  });
});

describe("pickCastProfile - evidence-driven entries from the prod beacon", () => {
  // MiTV-AFMU0 (an Amlogic Android TV). The "Dolby Audio" certification holds for
  // the TV's ANDROID apps, not for its cast runtime: serving E-AC-3 as
  // passthrough to the Default Media Receiver made 3 out of 3 casts fail in
  // prod on 2026-07-25 (TV stuck on the backdrop, zero audio segment
  // requested), while the SAME TV played the SAME files the day before when
  // the floor profile forced the AAC transcode. Same philosophy as the Ultra
  // entry: NEVER bet on a cast receiver's Dolby passthrough.
  it("never hands the Mi TV cast receiver Dolby passthrough - AAC is what plays", () => {
    const p = pickCastProfile("MiTV-AFMU0");
    expect(p.audioCodecs).not.toContain("eac3");
    expect(p.audioCodecs).not.toContain("ac3");
    expect(p.audioCodecs).toContain("aac");
  });

  it("keeps the Mi TV's video capabilities (HEVC 4K, HDR, h264 at 1080p)", () => {
    const p = pickCastProfile("MiTV-AFMU0");
    expect(p.videoCodecs).toContain("hevc");
    expect(p.hdr).toBe(true);
    expect(p.h264MaxHeight).toBe(1080);
  });
});

// Observed beats assumed: when the receiver reports what it probed (our
// custom receiver, CAF canDisplayType), the profile is those answers - no
// model table, no guess. The H.264 ceiling is the DIRECT probe when the
// receiver made one; the level is only a conservative fallback for a receiver
// built before that probe existed, and it is documented as such.
describe("profileFromReceiverCaps", () => {
  const observed = {
    videoCodecs: ["h264", "hevc", "vp9"], maxHeight: 2160, hdr: true, nativeDolby: false,
    maxAudioChannels: 2, maxH264Level: 42, h264MaxHeight: 1080,
  };

  it("takes the observed codecs, ceilings and HDR as they were answered", () => {
    expect(profileFromReceiverCaps(observed)).toEqual({
      videoCodecs: ["h264", "hevc", "vp9"],
      audioCodecs: ["aac", "mp3", "opus", "flac"],
      hdr: true, maxHeight: 2160, h264MaxHeight: 1080, maxAudioChannels: 2,
    });
  });

  it("the direct h264MaxHeight probe wins over the level, even when they disagree", () => {
    // A level-5.1 decoder string accepted but 4K H.264 refused at 3840x2160:
    // the resolution probe is the measured answer.
    expect(profileFromReceiverCaps({ ...observed, maxH264Level: 51, h264MaxHeight: 1080 }).h264MaxHeight).toBe(1080);
  });

  it("without the direct probe, the level is a conservative bound (documented fallback)", () => {
    expect(profileFromReceiverCaps({ ...observed, h264MaxHeight: undefined, maxH264Level: 51 }).h264MaxHeight).toBe(2160);
    expect(profileFromReceiverCaps({ ...observed, h264MaxHeight: undefined, maxH264Level: 42 }).h264MaxHeight).toBe(1080);
    expect(profileFromReceiverCaps({ ...observed, h264MaxHeight: undefined, maxH264Level: 31 }).h264MaxHeight).toBe(720);
    expect(h264HeightFromLevel(50)).toBe(2160);
    expect(h264HeightFromLevel(40)).toBe(1080);
    expect(h264HeightFromLevel(0)).toBe(720);
  });

  it("adds Dolby only when the device itself vouches for passthrough", () => {
    expect(profileFromReceiverCaps({ ...observed, nativeDolby: true }).audioCodecs).toEqual(["aac", "mp3", "opus", "flac", "eac3", "ac3"]);
  });

  it("an empty codec answer still declares the h264 floor", () => {
    expect(profileFromReceiverCaps({ ...observed, videoCodecs: [] }).videoCodecs).toEqual(["h264"]);
  });

  it("the observed channel ceiling rides the master; the model table declares none", () => {
    const url = buildCastStreamUrl(SRV, "m1", null, "tok", profileFromReceiverCaps(observed));
    expect(url).toContain("maxAudioChannels=2");
    expect(url).toContain("maxH264Height=1080");
    expect(buildCastStreamUrl(SRV, "m1", null, "tok", pickCastProfile("Chromecast Ultra"))).not.toContain("maxAudioChannels");
  });
});

// The sender's request-time choices ride the master as inputs the server
// bakes (the viewer's audio track, the gapless opt-in) - never a decision.
describe("buildCastStreamUrl extras", () => {
  it("bakes the audio track and the gapless opt-in on the master only", () => {
    const p = pickCastProfile("Chromecast");
    const master = buildCastStreamUrl(SRV, "m1", null, "tok", p, { audioTrack: 1, gapless: true });
    expect(master).toContain("audioTrack=1");
    expect(master).toContain("gapless=1");
    const direct = buildCastStreamUrl(SRV, "m1", { playMethod: "PLAY_METHOD_DIRECT_PLAY" }, "tok", p, { audioTrack: 1, gapless: true });
    expect(direct).toBe(`${SRV}/api/v1/media/m1/stream?token=tok`);
    expect(buildCastStreamUrl(SRV, "m1", null, "tok", p)).not.toContain("audioTrack=");
  });
});

// Observed caps beat the model table; the table beats nothing; nothing is
// the floor - the same answer for a timeout, a Default Media Receiver or a
// device a subnet probe found without a name.
describe("castProfileFor", () => {
  const observed = {
    videoCodecs: ["h264", "hevc"], maxHeight: 2160, hdr: false, nativeDolby: false,
    maxAudioChannels: 2, maxH264Level: 42, h264MaxHeight: 1080,
  };
  it("observed caps win over a model name", () => {
    expect(castProfileFor(observed, "Google TV Streamer")).toEqual(profileFromReceiverCaps(observed));
  });
  it("a model name alone reads the table", () => {
    expect(castProfileFor(null, "Chromecast Ultra")).toEqual(pickCastProfile("Chromecast Ultra"));
  });
  it("nothing known is the floor", () => {
    expect(castProfileFor(null, null)).toBe(FLOOR_PROFILE);
    expect(castProfileFor(undefined, "")).toBe(FLOOR_PROFILE);
  });
});
