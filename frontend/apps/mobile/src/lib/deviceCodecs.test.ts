import { describe, it, expect, vi, beforeEach } from "vitest";

// The module probes at load, so each case re-imports it under its own mocks.
const rn = vi.hoisted(() => ({
  Platform: { OS: "android" as string, select: (o: Record<string, unknown>) => o.android },
  NativeModules: {} as Record<string, unknown>,
}));
const caps = vi.hoisted(() => ({
  getMaxH264Level: vi.fn<() => Promise<number | null>>(),
  getAudioProbe: vi.fn<() => Promise<unknown>>(),
}));

vi.mock("react-native", () => ({ Platform: rn.Platform, NativeModules: rn.NativeModules }));
vi.mock("@evey/media-caps", async () => {
  const real = await vi.importActual<typeof import("@evey/media-caps")>("@evey/media-caps");
  return { ...real, getMaxH264Level: caps.getMaxH264Level, getAudioProbe: caps.getAudioProbe };
});

async function load() {
  vi.resetModules();
  const mod = await import("./deviceCodecs");
  // Let the fire-and-forget probe settle.
  await new Promise((r) => setTimeout(r, 0));
  await new Promise((r) => setTimeout(r, 0));
  return mod;
}

beforeEach(() => {
  rn.Platform.OS = "android";
  delete rn.NativeModules.MediaCaps;
  caps.getMaxH264Level.mockReset().mockResolvedValue(41);
  caps.getAudioProbe.mockReset().mockResolvedValue(null);
});

describe("deviceCodecs", () => {
  it("android without the native module declares the software floor, no HDR, but still caps the H.264 level", async () => {
    const m = await load();
    expect(m.getDeviceAudioCodecs()).toEqual(["aac", "mp3", "opus", "flac", "vorbis"]);
    expect(m.getDeviceHdrSupport()).toBe(false);
    expect(m.getDeviceMaxH264Level()).toBe(41);
    expect(m.getFmp4AudioPassthrough()).toEqual(["mp3"]);
  });

  it("android with the native module: the Dolby family needs the sink's yes, HDR follows hdr10/hlg", async () => {
    rn.NativeModules.MediaCaps = {
      getSupportedAudioCodecs: async () => ["aac", "ac3", "eac3", "opus"],
      getHdrTypes: async () => ["dolby-vision", "hlg"],
    };
    caps.getAudioProbe.mockResolvedValue({ directPlayback: { eac3: true }, directProfiles: [] });
    const m = await load();
    expect(m.getDeviceAudioCodecs()).toEqual(["aac", "opus", "eac3"]);
    expect(m.getDeviceHdrSupport()).toBe(true);
  });

  it("a native module that throws leaves the floor and SDR in place; a level probe failure stays level-blind", async () => {
    rn.NativeModules.MediaCaps = {
      getSupportedAudioCodecs: async () => {
        throw new Error("no bridge");
      },
      getHdrTypes: async () => {
        throw new Error("no bridge");
      },
    };
    caps.getMaxH264Level.mockRejectedValue(new Error("no bridge"));
    const m = await load();
    expect(m.getDeviceAudioCodecs()).toEqual(["aac", "mp3", "opus", "flac", "vorbis"]);
    expect(m.getDeviceHdrSupport()).toBe(false);
    expect(m.getDeviceMaxH264Level()).toBeUndefined();
  });

  it("dolby-vision alone is not HDR for the server's purposes", async () => {
    rn.NativeModules.MediaCaps = {
      getSupportedAudioCodecs: async () => [],
      getHdrTypes: async () => ["dolby-vision"],
    };
    const m = await load();
    expect(m.getDeviceHdrSupport()).toBe(false);
    // An empty decoder list keeps the floor.
    expect(m.getDeviceAudioCodecs()).toEqual(["aac", "mp3", "opus", "flac", "vorbis"]);
  });

  it("iOS declares AVPlayer's codecs, never probes, and takes no fMP4 passthrough", async () => {
    rn.Platform.OS = "ios";
    rn.NativeModules.MediaCaps = { getSupportedAudioCodecs: async () => ["aac"], getHdrTypes: async () => ["hdr10"] };
    const m = await load();
    expect(m.getDeviceAudioCodecs()).toEqual(["aac", "mp3", "flac", "ac3", "eac3"]);
    expect(m.getFmp4AudioPassthrough()).toEqual([]);
    expect(m.getDeviceHdrSupport()).toBe(false);
    expect(m.getDeviceMaxH264Level()).toBeUndefined();
    expect(caps.getMaxH264Level).not.toHaveBeenCalled();
  });
});
