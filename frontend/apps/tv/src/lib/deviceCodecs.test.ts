import { describe, it, expect } from "vitest";
import type { AudioProbe } from "@evey/media-caps";
import { resolveAudioCodecs, getFmp4AudioPassthrough, ANDROID_TV_BASELINE, APPLE_TV_CODECS } from "./deviceCodecs";

// Real-shaped probe fixtures (audio_caps_probe beacons): a decoder list plus
// what the SINK accepts. The sink is the only witness that counts for the
// Dolby/DTS family - same rule as apps/mobile, see @evey/media-caps.
function probe(decoders: string[], directPlayback: Record<string, boolean>): AudioProbe {
  return {
    apiLevel: 34,
    isTv: true,
    deviceModel: "box",
    decoders,
    directPlayback,
    directProfiles: [],
    hdmi: { available: true },
    surroundOutputRaw: -1,
    outputDeviceTypes: [],
  };
}

describe("resolveAudioCodecs", () => {
  it("falls back to the baseline when no probe of any kind is installed", () => {
    expect(resolveAudioCodecs(ANDROID_TV_BASELINE, { status: "unavailable" }, null)).toBe(
      ANDROID_TV_BASELINE,
    );
  });

  it("falls back to the baseline when the legacy probe throws and no sink probe exists", () => {
    expect(resolveAudioCodecs(ANDROID_TV_BASELINE, { status: "error" }, null)).toBe(
      ANDROID_TV_BASELINE,
    );
  });

  it("falls back to the baseline when both probes come back empty", () => {
    expect(
      resolveAudioCodecs(ANDROID_TV_BASELINE, { status: "ok", codecs: [] }, probe([], {})),
    ).toBe(ANDROID_TV_BASELINE);
  });

  it("declares the box's real codecs, without Dolby, when no Dolby decoder is available", () => {
    const codecs = ["aac", "mp3", "opus"];
    expect(resolveAudioCodecs(ANDROID_TV_BASELINE, { status: "ok", codecs }, null)).toEqual(codecs);
  });

  it("takes the decoder list from the sink probe when the legacy module is absent (the TV build)", () => {
    const p = probe(["aac", "mp3", "opus", "flac"], {});
    expect(resolveAudioCodecs(ANDROID_TV_BASELINE, { status: "unavailable" }, p)).toEqual([
      "aac",
      "mp3",
      "opus",
      "flac",
    ]);
  });

  it("declares ac3/eac3 only when the sink takes them, even with a decoder present", () => {
    const decoders = ["aac", "mp3", "ac3", "eac3"];
    // decoder + sink refuses (the choppy-tablet shape): the family stays out
    expect(
      resolveAudioCodecs(ANDROID_TV_BASELINE, { status: "unavailable" }, probe(decoders, { ac3: false, eac3: false })),
    ).toEqual(["aac", "mp3"]);
    // decoder + sink accepts (box on an AVR): declared
    expect(
      resolveAudioCodecs(ANDROID_TV_BASELINE, { status: "unavailable" }, probe(decoders, { ac3: true, eac3: true })),
    ).toEqual(["aac", "mp3", "ac3", "eac3"]);
  });

  it("declares ac3 on a passthrough box that has no decoder at all", () => {
    const p = probe(["aac", "mp3"], { ac3: true, eac3: false });
    expect(resolveAudioCodecs(ANDROID_TV_BASELINE, { status: "unavailable" }, p)).toEqual([
      "aac",
      "mp3",
      "ac3",
    ]);
  });

  it("strips the family when a legacy decoder list arrives without any sink evidence", () => {
    const codecs = ["aac", "mp3", "opus", "flac", "ac3", "eac3"];
    expect(resolveAudioCodecs(ANDROID_TV_BASELINE, { status: "ok", codecs }, null)).toEqual([
      "aac",
      "mp3",
      "opus",
      "flac",
    ]);
  });

  it("never includes ac3/eac3 in the Android TV baseline itself", () => {
    expect(ANDROID_TV_BASELINE).not.toContain("ac3");
    expect(ANDROID_TV_BASELINE).not.toContain("eac3");
  });

  it("keeps ac3/eac3 in the Apple TV baseline (AVPlayer decodes them on every device we support)", () => {
    expect(APPLE_TV_CODECS).toContain("ac3");
    expect(APPLE_TV_CODECS).toContain("eac3");
    // and no probe on tvOS means the baseline goes through untouched
    expect(resolveAudioCodecs(APPLE_TV_CODECS, { status: "unavailable" }, null)).toBe(APPLE_TV_CODECS);
  });
});

// The react-native alias in vitest.config.ts reports Platform.OS "android", so
// this file covers the Android TV half. The Apple TV half needs its own module
// mock and lives in fmp4AudioPassthrough.appletv.test.ts.
describe("getFmp4AudioPassthrough (Android TV)", () => {
  // media3/ExoPlayer reads MP3 back out of an fMP4 `mp4a` sample entry, so the
  // server may COPY an old rip's MP3 track instead of cascading it through its
  // AAC encoder (20.3 dB of coding noise under dense music, measured 2026-09-06).
  it("declares mp3 so the server can pass the track through", () => {
    expect(getFmp4AudioPassthrough()).toEqual(["mp3"]);
  });

  // The opt-in selects from what the server can mux and advertise; anything
  // else here would be refused server-side anyway.
  it("declares mp3 and nothing else", () => {
    expect(getFmp4AudioPassthrough()).toHaveLength(1);
  });

  // Two different questions. Conflating them is what shipped the re-encode on
  // every client: the baseline decodes mp3, which says nothing about fMP4.
  it("stays separate from the decoder baseline, which also lists mp3", () => {
    expect(ANDROID_TV_BASELINE).toContain("mp3");
    expect(APPLE_TV_CODECS).toContain("mp3");
    expect(getFmp4AudioPassthrough()).not.toBe(ANDROID_TV_BASELINE);
  });
});
