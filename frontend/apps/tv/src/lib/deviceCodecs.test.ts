import { describe, it, expect } from "vitest";
import { resolveAudioCodecs, ANDROID_TV_BASELINE, APPLE_TV_CODECS } from "./deviceCodecs";

// Pure resolution covers every native-probe outcome without touching
// NativeModules/Platform: this is the part usePlaySession actually depends on.
describe("resolveAudioCodecs", () => {
  it("falls back to the baseline when no probe module is installed", () => {
    expect(resolveAudioCodecs(ANDROID_TV_BASELINE, { status: "unavailable" })).toBe(
      ANDROID_TV_BASELINE,
    );
  });

  it("falls back to the baseline when the probe throws", () => {
    expect(resolveAudioCodecs(ANDROID_TV_BASELINE, { status: "error" })).toBe(ANDROID_TV_BASELINE);
  });

  it("falls back to the baseline when the probe resolves with an empty list", () => {
    expect(
      resolveAudioCodecs(ANDROID_TV_BASELINE, { status: "ok", codecs: [] }),
    ).toBe(ANDROID_TV_BASELINE);
  });

  it("declares the box's real codecs, without Dolby, when the probe says no Dolby decoder is available", () => {
    const codecs = ["aac", "mp3", "opus"];
    expect(resolveAudioCodecs(ANDROID_TV_BASELINE, { status: "ok", codecs })).toEqual(codecs);
  });

  it("declares ac3/eac3 when the probe confirms the box decodes or passes them through", () => {
    const codecs = ["aac", "mp3", "opus", "flac", "ac3", "eac3"];
    expect(resolveAudioCodecs(ANDROID_TV_BASELINE, { status: "ok", codecs })).toEqual(codecs);
  });

  it("never includes ac3/eac3 in the Android TV baseline itself", () => {
    expect(ANDROID_TV_BASELINE).not.toContain("ac3");
    expect(ANDROID_TV_BASELINE).not.toContain("eac3");
  });

  it("keeps ac3/eac3 in the Apple TV baseline (AVPlayer decodes them on every device we support)", () => {
    expect(APPLE_TV_CODECS).toContain("ac3");
    expect(APPLE_TV_CODECS).toContain("eac3");
  });
});
