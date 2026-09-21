import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { getBrowserCapabilities } from "./codec";

describe("canPlayHevc", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  it("returns false when HEVC not supported", async () => {
    vi.spyOn(HTMLVideoElement.prototype, "canPlayType").mockReturnValue("");
    const { canPlayHevc } = await import("./codec");
    expect(canPlayHevc()).toBe(false);
  });

  it("returns true when HEVC probably supported", async () => {
    vi.spyOn(HTMLVideoElement.prototype, "canPlayType").mockReturnValue("probably");
    const { canPlayHevc } = await import("./codec");
    expect(canPlayHevc()).toBe(true);
  });

  it("returns false when canPlayType returns maybe", async () => {
    vi.spyOn(HTMLVideoElement.prototype, "canPlayType").mockReturnValue("maybe");
    const { canPlayHevc } = await import("./codec");
    expect(canPlayHevc()).toBe(false);
  });
});

describe("getBrowserCapabilities", () => {
  it("includes h264 when mp4/avc supported", () => {
    vi.spyOn(HTMLVideoElement.prototype, "canPlayType").mockImplementation((type) => {
      if (type.includes("avc1")) return "probably";
      if (type === "video/mp4") return "probably";
      return "";
    });
    const caps = getBrowserCapabilities();
    expect(caps.videoCodecs).toContain("h264");
    expect(caps.containers).toContain("mp4");
  });

  it("includes vp9 when webm/vp9 supported", () => {
    vi.spyOn(HTMLVideoElement.prototype, "canPlayType").mockImplementation((type) => {
      if (type.includes("vp9")) return "maybe";
      if (type === "video/webm") return "maybe";
      return "";
    });
    const caps = getBrowserCapabilities();
    expect(caps.videoCodecs).toContain("vp9");
    expect(caps.containers).toContain("webm");
  });

  it("includes av1 when av01 codec supported", () => {
    vi.spyOn(HTMLVideoElement.prototype, "canPlayType").mockImplementation((type) => {
      if (type.includes("av01")) return "probably";
      return "";
    });
    const caps = getBrowserCapabilities();
    expect(caps.videoCodecs).toContain("av1");
  });

  it("always includes aac, mp3, opus in audioCodecs", () => {
    vi.spyOn(HTMLVideoElement.prototype, "canPlayType").mockReturnValue("");
    const caps = getBrowserCapabilities();
    expect(caps.audioCodecs).toContain("aac");
    expect(caps.audioCodecs).toContain("mp3");
    expect(caps.audioCodecs).toContain("opus");
  });

  it("includes ac3 when ac-3 supported", () => {
    vi.spyOn(HTMLVideoElement.prototype, "canPlayType").mockImplementation((type) => {
      if (type.includes("ac-3")) return "probably";
      return "";
    });
    const caps = getBrowserCapabilities();
    expect(caps.audioCodecs).toContain("ac3");
  });

  it("includes eac3 when ec-3 supported", () => {
    vi.spyOn(HTMLVideoElement.prototype, "canPlayType").mockImplementation((type) => {
      if (type.includes("ec-3")) return "probably";
      return "";
    });
    const caps = getBrowserCapabilities();
    expect(caps.audioCodecs).toContain("eac3");
  });

  it("includes flac when audio/flac supported", () => {
    vi.spyOn(HTMLVideoElement.prototype, "canPlayType").mockImplementation((type) => {
      if (type === "audio/flac") return "probably";
      return "";
    });
    const caps = getBrowserCapabilities();
    expect(caps.audioCodecs).toContain("flac");
  });

  it("returns empty videoCodecs and containers when nothing is supported", () => {
    vi.spyOn(HTMLVideoElement.prototype, "canPlayType").mockReturnValue("");
    const caps = getBrowserCapabilities();
    expect(caps.videoCodecs).toHaveLength(0);
    expect(caps.containers).toHaveLength(0);
  });
});

describe("canDisplayHdr", () => {
  const originalMatchMedia = window.matchMedia;
  beforeEach(() => {
    vi.resetModules();
  });
  afterEach(() => {
    window.matchMedia = originalMatchMedia;
  });

  it("is true when the display reports dynamic-range: high", async () => {
    window.matchMedia = vi.fn().mockReturnValue({ matches: true }) as unknown as typeof window.matchMedia;
    const { canDisplayHdr, getBrowserCapabilities } = await import("./codec");
    expect(canDisplayHdr).toBe(true);
    expect(getBrowserCapabilities().hdr).toBe(true);
  });

  it("is false on an SDR display", async () => {
    window.matchMedia = vi.fn().mockReturnValue({ matches: false }) as unknown as typeof window.matchMedia;
    const { canDisplayHdr, getBrowserCapabilities } = await import("./codec");
    expect(canDisplayHdr).toBe(false);
    expect(getBrowserCapabilities().hdr).toBe(false);
  });

  it("is false (fail-safe) when matchMedia throws", async () => {
    window.matchMedia = vi.fn(() => {
      throw new Error("matchMedia unavailable");
    }) as unknown as typeof window.matchMedia;
    const { canDisplayHdr } = await import("./codec");
    expect(canDisplayHdr).toBe(false);
  });
});
