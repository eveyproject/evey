import { describe, it, expect, vi, beforeEach } from "vitest";

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
