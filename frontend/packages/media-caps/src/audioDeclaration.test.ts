import { describe, it, expect } from "vitest";
import { declarableAudioCodecs, sinkAcceptsCodec } from "./audioDeclaration";
import type { AudioProbe } from "./audioProbe";

function probe(overrides: Partial<AudioProbe> = {}): AudioProbe {
  return {
    apiLevel: 34,
    isTv: false,
    deviceModel: "generic",
    decoders: ["aac", "mp3", "flac"],
    directPlayback: {},
    directProfiles: [],
    hdmi: { available: false },
    surroundOutputRaw: -1,
    outputDeviceTypes: [2],
    ...overrides,
  };
}

// The two real beacons of 2026-08-31, verbatim. The a tablet brand is the tablet whose
// direct-played AC-3 renders choppy by stretches: it DECODES ac3/eac3 but its
// sink only takes dts - the decoder was answering the wrong question.
const TABLET = probe({
  deviceModel: "a tablet brand 23043RP34G",
  decoders: ["aac", "mp3", "opus", "flac", "vorbis", "ac3", "eac3"],
  directPlayback: { dts: true, dts_hd: true, ac3: false, eac3: false },
  directProfiles: [2, 4, 21, 22, 9, 10, 11, 12, 7, 8, 106, 107, 108].map((encoding) => ({ encoding, channelMasks: [] })),
});
const PHONE_NO_DOLBY = probe({
  deviceModel: "Phone P2",
  apiLevel: 35,
  decoders: ["aac", "mp3", "opus", "flac", "vorbis"],
  directPlayback: { ac3: false, eac3: false, dts: false },
});

describe("declarableAudioCodecs", () => {
  it("strips ac3/eac3 from the a tablet brand tablet whose sink refuses them - the reported stutter", () => {
    const declared = declarableAudioCodecs(TABLET.decoders, TABLET);
    expect(declared).not.toContain("ac3");
    expect(declared).not.toContain("eac3");
    // Its sink genuinely takes DTS bitstream (dp + profiles 7/8): passthrough
    // works there even though no DTS decoder is listed.
    expect(declared).toContain("dts");
    expect(declared).toEqual(expect.arrayContaining(["aac", "mp3", "opus", "flac", "vorbis"]));
  });

  it("declares only the software baseline on a phone with no direct playback at all", () => {
    expect(declarableAudioCodecs(PHONE_NO_DOLBY.decoders, PHONE_NO_DOLBY)).toEqual(["aac", "mp3", "opus", "flac", "vorbis"]);
  });

  it("declares the family on a passthrough box even WITHOUT a decoder", () => {
    const box = probe({ isTv: true, decoders: ["aac", "mp3"], directPlayback: { ac3: true, eac3: true } });
    const declared = declarableAudioCodecs(box.decoders, box);
    expect(declared).toContain("ac3");
    expect(declared).toContain("eac3");
  });

  it("accepts API 33 directProfiles as the sink's yes (E-AC3 as JOC included)", () => {
    const p = probe({ directProfiles: [{ encoding: 18, channelMasks: [63] }] });
    expect(sinkAcceptsCodec(p, "eac3")).toBe(true);
    expect(sinkAcceptsCodec(p, "ac3")).toBe(false);
  });

  it("declares none of the family without a probe - one transcode too many beats a choppy film", () => {
    expect(declarableAudioCodecs(["aac", "ac3", "eac3", "truehd", "flac"], null)).toEqual(["aac", "flac"]);
  });

  // The phone's own ac3go renderer decodes these: neither the a tablet brand's refusing
  // sink nor a phone with no Dolby decoder at all holds them back.
  it("declares what the player decodes itself, whatever the decoders or the sink say", () => {
    for (const p of [TABLET, PHONE_NO_DOLBY]) {
      const declared = declarableAudioCodecs(p.decoders, p, ["ac3", "eac3"]);
      expect(declared).toContain("ac3");
      expect(declared).toContain("eac3");
      expect(declared).not.toContain("truehd");
    }
    expect(declarableAudioCodecs(["aac"], null, ["ac3", "eac3"])).toEqual(["aac", "ac3", "eac3"]);
  });
});
