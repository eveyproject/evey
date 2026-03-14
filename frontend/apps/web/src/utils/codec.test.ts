import { describe, it, expect, vi, beforeEach } from "vitest";
import { getBrowserCapabilities } from "./codec";

describe("canPlayHevc", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  it("returns false when HEVC not supported", async () => {
    vi.spyOn(HTMLVideoElement.prototype, "canPlayType").mockReturnValue("");
    const { canPlayHevc } = await import("./codec");
    expect(canPlayHevc).toBe(false);
  });

  it("returns true when HEVC probably supported", async () => {
    vi.spyOn(HTMLVideoElement.prototype, "canPlayType").mockReturnValue("probably");
    const { canPlayHevc } = await import("./codec");
    expect(canPlayHevc).toBe(true);
  });

  it("returns false when canPlayType returns maybe", async () => {
    vi.spyOn(HTMLVideoElement.prototype, "canPlayType").mockReturnValue("maybe");
    const { canPlayHevc } = await import("./codec");
    expect(canPlayHevc).toBe(false);
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

  it("returns empty videoCodecs and containers when nothing is supported", () => {
    vi.spyOn(HTMLVideoElement.prototype, "canPlayType").mockReturnValue("");
    const caps = getBrowserCapabilities();
    expect(caps.videoCodecs).toHaveLength(0);
    expect(caps.containers).toHaveLength(0);
  });
});
