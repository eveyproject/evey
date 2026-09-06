import { describe, it, expect, vi } from "vitest";

// The iOS half of the fMP4 audio-passthrough opt-in - see the android file.
vi.mock("react-native", () => ({
  Platform: { OS: "ios" },
  NativeModules: {},
}));

vi.mock("@evey/media-caps", () => ({
  getAudioProbe: vi.fn().mockResolvedValue(null),
  declarableAudioCodecs: (decoders: string[]) => decoders,
}));

describe("getFmp4AudioPassthrough (ios)", () => {
  // AVPlayer does not take MP3 inside fMP4 at all. Declaring it here would make
  // the server advertise mp4a.6B and serve MP3 samples to a player that refuses
  // the whole rendition - silence, not degraded audio. iOS keeps the AAC
  // re-encode on purpose.
  it("declares nothing, so iOS keeps the server-side AAC re-encode", async () => {
    const { getFmp4AudioPassthrough } = await import("./deviceCodecs");
    expect(getFmp4AudioPassthrough()).toEqual([]);
  });

  // IOS_CODECS lists mp3 because AVPlayer decodes .mp3 files. That is exactly
  // the declaration this rule must NOT be derived from.
  it("does not follow the decoder list, which does contain mp3", async () => {
    const { getFmp4AudioPassthrough, getDeviceAudioCodecs } = await import("./deviceCodecs");
    expect(getDeviceAudioCodecs()).toContain("mp3");
    expect(getFmp4AudioPassthrough()).toEqual([]);
  });
});
