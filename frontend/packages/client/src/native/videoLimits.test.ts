import { describe, expect, it } from "vitest";
import { decoderMaxHeight, decoderMaxWidth, devicePixelFormats, tighterOf } from "./videoLimits";

const chromecastHd = { maxWidth: 1920, maxHeight: 1088, tenBitCodecs: [] as string[], codecs: { h264: { maxWidth: 1920, maxHeight: 1088 }, hevc: { maxWidth: 1920, maxHeight: 1088 } } };
const box4k = { maxWidth: 4096, maxHeight: 2304, tenBitCodecs: ["hevc", "vp9"], codecs: {} };

describe("devicePixelFormats", () => {
  it("declares 8-bit for every offered codec and 10-bit only where the decoder takes it", () => {
    expect(devicePixelFormats(chromecastHd, ["h264", "hevc", "vp9"])).toEqual(["h264:yuv420p", "hevc:yuv420p", "vp9:yuv420p"]);
    expect(devicePixelFormats(box4k, ["h264", "hevc", "vp9"])).toEqual(["h264:yuv420p", "hevc:yuv420p", "hevc:yuv420p10", "vp9:yuv420p", "vp9:yuv420p10"]);
  });
  it("declares nothing when the limits are unknown, keeping the server's rule", () => {
    expect(devicePixelFormats(undefined, ["h264", "hevc"])).toBeUndefined();
  });
});

describe("decoder ceilings", () => {
  it("passes a plausible ceiling and drops an empty probe", () => {
    expect(decoderMaxHeight(chromecastHd)).toBe(1088);
    expect(decoderMaxWidth(chromecastHd)).toBe(1920);
    expect(decoderMaxHeight({ ...box4k, maxHeight: 0 })).toBeUndefined();
    expect(decoderMaxHeight(undefined)).toBeUndefined();
  });
  it("keeps the tighter of the viewer's pick and the decoder's limit", () => {
    expect(tighterOf(720, 1088)).toBe(720);
    expect(tighterOf(undefined, 1088)).toBe(1088);
    expect(tighterOf(2160, undefined)).toBe(2160);
    expect(tighterOf(undefined, undefined)).toBeUndefined();
  });
});
