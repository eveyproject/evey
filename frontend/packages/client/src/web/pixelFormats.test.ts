import { describe, it, expect } from "vitest";
import { baselinePixelFormats, detectPixelFormats } from "./pixelFormats";

// Chrome on a desktop, as measured: VP9 profile 3 (4:4:4 10-bit) supported and
// smooth, AV1 4:4:4 supported but not smooth, 10-bit H.264 unsupported.
const chrome = async (config: MediaDecodingConfiguration) => {
  const type = config.video?.contentType ?? "";
  if (type.includes("vp09.03") || type.includes("vp09.01")) return { supported: true, smooth: true };
  if (type.includes("av01.1")) return { supported: true, smooth: false };
  return { supported: false, smooth: false };
};

describe("pixel formats a browser declares", () => {
  it("restates the server's 4:2:0 rule per codec", () => {
    expect(baselinePixelFormats(["h264", "vp9"])).toEqual(["h264:yuv420p", "vp9:yuv420p", "vp9:yuv420p10"]);
  });

  it("adds only what decodes smoothly, after the rule", async () => {
    expect(await detectPixelFormats(["h264", "vp9", "av1"], chrome)).toEqual([
      "h264:yuv420p", "vp9:yuv420p", "vp9:yuv420p10", "av1:yuv420p", "av1:yuv420p10",
      "vp9:yuv444p", "vp9:yuv444p10",
    ]);
  });

  it("declares nothing when nothing goes past the rule, so the URL stays as it was", async () => {
    const none = async () => ({ supported: true, smooth: false });
    expect(await detectPixelFormats(["h264", "vp9"], none)).toBeUndefined();
  });

  it("never asks about a codec the browser does not play, and survives a throwing engine", async () => {
    const asked: string[] = [];
    const throwing = async (config: MediaDecodingConfiguration) => {
      asked.push(config.video?.contentType ?? "");
      throw new TypeError("unsupported codec string");
    };
    expect(await detectPixelFormats(["h264"], throwing)).toBeUndefined();
    expect(asked.every((t) => t.includes("avc1"))).toBe(true);
  });
});
