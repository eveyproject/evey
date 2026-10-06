import { describe, it, expect, vi, beforeEach } from "vitest";

// The module probes at load, so each case re-imports it under its own mocks.
const rn = vi.hoisted(() => ({
  Platform: { OS: "android" as string, select: (o: Record<string, unknown>) => o.android },
  NativeModules: {} as Record<string, unknown>,
}));
const caps = vi.hoisted(() => ({
  getMaxH264Level: vi.fn<() => Promise<number | null>>(),
  getAudioProbe: vi.fn<() => Promise<unknown>>(),
  // The shared display probe. Null is "not in this build", which is what sends
  // the module to the legacy old-bridge fallback below.
  getDisplayHdrTypes: vi.fn<() => Promise<string[] | null>>(),
  getPlayerAudioDecoders: vi.fn<() => Promise<string[]>>(),
  // The launch check of ac3go; null is a build without it.
  checkPlaybackDecoder: vi.fn<() => Promise<unknown>>(),
}));

vi.mock("react-native", () => ({ Platform: rn.Platform, NativeModules: rn.NativeModules }));
vi.mock("@evey/media-caps", async () => {
  const real = await vi.importActual<typeof import("@evey/media-caps")>("@evey/media-caps");
  // displaySupportsHdr stays REAL: the rule for what a display list means is
  // shared with the TV, and a hand-rolled copy here would let the two drift.
  return {
    ...real,
    getMaxH264Level: caps.getMaxH264Level,
    getAudioProbe: caps.getAudioProbe,
    getDisplayHdrTypes: caps.getDisplayHdrTypes,
    getPlayerAudioDecoders: caps.getPlayerAudioDecoders,
    checkPlaybackDecoder: caps.checkPlaybackDecoder,
  };
});

async function load() {
  vi.resetModules();
  const mod = await import("./deviceCodecs");
  // Let the fire-and-forget probe settle. Several turns: the probe awaits the
  // codec list, the sink probe, the shared display probe and - when that one
  // answers null - the legacy module behind it.
  for (let i = 0; i < 6; i++) await new Promise((r) => setTimeout(r, 0));
  return mod;
}

beforeEach(() => {
  rn.Platform.OS = "android";
  delete rn.NativeModules.MediaCaps;
  caps.getMaxH264Level.mockReset().mockResolvedValue(41);
  caps.getAudioProbe.mockReset().mockResolvedValue(null);
  caps.getDisplayHdrTypes.mockReset().mockResolvedValue(null);
  caps.getPlayerAudioDecoders.mockReset().mockResolvedValue([]);
  caps.checkPlaybackDecoder.mockReset().mockResolvedValue(null);
});

describe("deviceCodecs", () => {
  it("android without the native module declares the software floor, no HDR, but still caps the H.264 level", async () => {
    const m = await load();
    expect(m.getDeviceAudioCodecs()).toEqual(["aac", "mp3", "opus", "flac", "vorbis"]);
    expect(m.getDeviceHdrSupport()).toBe(false);
    expect(m.getDeviceMaxH264Level()).toBe(41);
    expect(m.getFmp4AudioPassthrough()).toEqual(["mp3", "opus"]);
  });

  it("android TV with the native module: the Dolby family needs the sink's yes, HDR follows hdr10/hlg", async () => {
    rn.NativeModules.MediaCaps = {
      getSupportedAudioCodecs: async () => ["aac", "ac3", "eac3", "opus"],
      getHdrTypes: async () => ["dolby-vision", "hlg"],
    };
    caps.getAudioProbe.mockResolvedValue({ isTv: true, directPlayback: { eac3: true }, directProfiles: [] });
    const m = await load();
    expect(m.getDeviceAudioCodecs()).toEqual(["aac", "opus", "eac3"]);
    expect(m.getDeviceHdrSupport()).toBe(true);
  });

  // The phone build: ac3go decodes the Dolby family in the player, so a sink
  // that refuses it (every phone speaker and headset) no longer matters.
  it("android with the ac3go player decoder declares ac3/eac3 despite a refusing sink", async () => {
    rn.NativeModules.MediaCaps = { getSupportedAudioCodecs: async () => ["aac", "ac3", "eac3", "opus"] };
    caps.getAudioProbe.mockResolvedValue({ directPlayback: {}, directProfiles: [] });
    caps.getPlayerAudioDecoders.mockResolvedValue(["ac3", "eac3"]);
    expect((await load()).getDeviceAudioCodecs()).toEqual(["aac", "opus", "ac3", "eac3"]);
  });

  // 2026-10-01, a Chromecast HD: too slow for ac3go, the app stopped mid-episode.
  // The launch check's verdict wins over the module's plain list.
  it("does not declare ac3go when the launch check blocks it, and reports the check", async () => {
    rn.NativeModules.MediaCaps = { getSupportedAudioCodecs: async () => ["aac", "opus"] };
    caps.getAudioProbe.mockResolvedValue({ directPlayback: {}, directProfiles: [] });
    caps.getPlayerAudioDecoders.mockResolvedValue(["ac3", "eac3"]);
    const check = { decoders: [], blockedBy: "bench", bench: { rtf: 0.6, slow: true }, abnormalExit: null };
    caps.checkPlaybackDecoder.mockResolvedValue(check);
    const m = await load();
    expect(m.getDeviceAudioCodecs()).toEqual(["aac", "opus"]);
    expect(m.getPlayerAudioCodecs()).toEqual([]);
    expect(await m.getDecoderCheck()).toBe(check);
  });

  it("drops ac3go from the declaration when the player's watchdog gives up on it", async () => {
    rn.NativeModules.MediaCaps = { getSupportedAudioCodecs: async () => ["aac", "opus"] };
    caps.getAudioProbe.mockResolvedValue({ directPlayback: {}, directProfiles: [] });
    caps.checkPlaybackDecoder.mockResolvedValue({ decoders: ["ac3", "eac3"], blockedBy: "", bench: null, abnormalExit: null });
    const m = await load();
    expect(m.getDeviceAudioCodecs()).toEqual(["aac", "opus", "ac3", "eac3"]);
    m.dropPlayerAudioCodecs();
    expect(m.getDeviceAudioCodecs()).toEqual(["aac", "opus"]);
    expect(m.getPlayerAudioCodecs()).toEqual([]);
  });

  // The shared probe is the one the TV reads too; the legacy old-bridge module
  // is only the fallback for builds that predate it. It also wins outright, so
  // the two never have to agree.
  it("prefers the shared display probe over the legacy module", async () => {
    rn.NativeModules.MediaCaps = {
      getSupportedAudioCodecs: async () => ["aac"],
      getHdrTypes: async () => ["hdr10"],
    };
    caps.getDisplayHdrTypes.mockResolvedValue([]);
    expect((await load()).getDeviceHdrSupport()).toBe(false);

    caps.getDisplayHdrTypes.mockResolvedValue(["hdr10plus"]);
    expect((await load()).getDeviceHdrSupport()).toBe(true);
  });

  // Dolby Vision alone is not an HDR10 declaration: the server carries one bool
  // and reads it as HDR10-capable, and tone-maps DoVi sources regardless.
  it("stays SDR for a Dolby-Vision-only panel", async () => {
    caps.getDisplayHdrTypes.mockResolvedValue(["dovi"]);
    expect((await load()).getDeviceHdrSupport()).toBe(false);
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
