import { describe, it, expect, vi } from "vitest";

// The Apple TV half of the fMP4 audio-passthrough opt-in. Separate file because
// Platform.OS comes from a module-level mock (the vitest alias reports
// "android"), and the whole point of this rule is that the two engines answer
// DIFFERENTLY. The Android TV half lives in deviceCodecs.test.ts.
vi.mock("react-native", () => ({
  Platform: { OS: "ios", select: (obj: Record<string, unknown>) => obj.ios },
  NativeModules: {},
  Dimensions: { get: () => ({ width: 1920, height: 1080, scale: 1 }) },
}));

vi.mock("@evey/media-caps", () => ({
  getAudioProbe: vi.fn().mockResolvedValue(null),
  declarableAudioCodecs: (decoders: string[]) => decoders,
}));

describe("getFmp4AudioPassthrough (Apple TV)", () => {
  // tvOS plays our HLS through AVPlayer, which does not take MP3 in fMP4 at
  // all. Declaring it would not degrade the audio, it would make AVPlayer
  // refuse the rendition outright: silence. Apple TV keeps the AAC re-encode.
  it("declares nothing, so tvOS keeps the server-side AAC re-encode", async () => {
    const { getFmp4AudioPassthrough } = await import("./deviceCodecs");
    expect(getFmp4AudioPassthrough()).toEqual([]);
  });

  // APPLE_TV_CODECS lists mp3 because AVPlayer decodes .mp3 files - exactly the
  // declaration this rule must NOT be derived from.
  it("does not follow the decoder list, which does contain mp3", async () => {
    const { getFmp4AudioPassthrough, APPLE_TV_CODECS } = await import("./deviceCodecs");
    expect(APPLE_TV_CODECS).toContain("mp3");
    expect(getFmp4AudioPassthrough()).toEqual([]);
  });
});
