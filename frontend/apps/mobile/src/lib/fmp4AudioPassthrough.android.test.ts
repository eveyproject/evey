import { describe, it, expect, vi } from "vitest";

// The Android half of the fMP4 audio-passthrough opt-in. Split across two files
// because Platform.OS is decided by a module-level vi.mock, and the whole point
// of this rule is that the two platforms answer DIFFERENTLY.
vi.mock("react-native", () => ({
  Platform: { OS: "android" },
  // deviceCodecs reads NativeModules.MediaCaps at import time; absent = the
  // conservative baseline, and the async probe returns before touching
  // @evey/media-caps.
  NativeModules: {},
}));

vi.mock("@evey/media-caps", () => ({
  getAudioProbe: vi.fn().mockResolvedValue(null),
  declarableAudioCodecs: (decoders: string[]) => decoders,
  getPlayerAudioDecoders: async () => [],
}));

describe("getFmp4AudioPassthrough (android)", () => {
  // media3/ExoPlayer reads MP3 back out of an fMP4 `mp4a` sample entry, so the
  // server can COPY an old rip's MP3 track instead of cascading it through its
  // AAC encoder (prod 2026-09-06: 20.3 dB of coding noise under dense music).
  it("declares mp3 so the server can pass the track through", async () => {
    const { getFmp4AudioPassthrough } = await import("./deviceCodecs");
    expect(getFmp4AudioPassthrough()).toEqual(["mp3"]);
  });

  // The opt-in is about the CONTAINER, not the decoder. Anything else declared
  // here would be a codec nobody proved the muxer and the RFC 6381 attribute
  // can carry, and the server would refuse it anyway.
  it("declares mp3 and nothing else", async () => {
    const { getFmp4AudioPassthrough } = await import("./deviceCodecs");
    expect(getFmp4AudioPassthrough()).toHaveLength(1);
  });

  // Two different questions, and conflating them is what shipped the bug on
  // every web and iOS client: they all decode mp3, none of them take it in fMP4.
  it("stays separate from the decoder list", async () => {
    const { getFmp4AudioPassthrough, getDeviceAudioCodecs } = await import("./deviceCodecs");
    expect(getDeviceAudioCodecs()).toContain("mp3");
    expect(getFmp4AudioPassthrough()).not.toBe(getDeviceAudioCodecs());
  });
});
