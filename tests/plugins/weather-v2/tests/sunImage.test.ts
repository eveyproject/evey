import { describe, it, expect } from "vitest";
import { meanLuma, isUsableSunFrame, SUN_FRAME_MIN_LUMA } from "../src/domain/services/sunImage";

// Build an RGBA buffer filled with one color.
function rgba(r: number, g: number, b: number, pixels: number): Uint8ClampedArray {
  const buf = new Uint8ClampedArray(pixels * 4);
  for (let i = 0; i < pixels; i++) {
    buf[i * 4] = r;
    buf[i * 4 + 1] = g;
    buf[i * 4 + 2] = b;
    buf[i * 4 + 3] = 255;
  }
  return buf;
}

describe("meanLuma", () => {
  it("is 0 for an all-black buffer and 1 for an all-white buffer", () => {
    expect(meanLuma(rgba(0, 0, 0, 64))).toBe(0);
    expect(meanLuma(rgba(255, 255, 255, 64))).toBeCloseTo(1, 5);
  });

  it("is ~0.5 for mid gray", () => {
    expect(meanLuma(rgba(128, 128, 128, 64))).toBeCloseTo(0.5, 1);
  });

  it("weights green above red above blue (Rec. 709)", () => {
    const g = meanLuma(rgba(0, 255, 0, 16));
    const r = meanLuma(rgba(255, 0, 0, 16));
    const b = meanLuma(rgba(0, 0, 255, 16));
    expect(g).toBeGreaterThan(r);
    expect(r).toBeGreaterThan(b);
  });

  it("ignores the alpha channel", () => {
    const opaque = rgba(200, 150, 40, 32);
    const transparent = rgba(200, 150, 40, 32);
    for (let i = 3; i < transparent.length; i += 4) transparent[i] = 0;
    expect(meanLuma(transparent)).toBe(meanLuma(opaque));
  });

  it("returns 0 for an empty buffer instead of NaN", () => {
    expect(meanLuma(new Uint8ClampedArray(0))).toBe(0);
  });
});

describe("isUsableSunFrame", () => {
  it("rejects an SDO dark frame (black disc, caption text only)", () => {
    // Real failure mode (2026-07-22): SDO served an all-black AIA 171 frame
    // during an eclipse/calibration window - a valid JPEG whose only bright
    // pixels are the white caption. Mean luma of such a frame is ~0.004.
    expect(isUsableSunFrame(0.004)).toBe(false);
  });

  it("accepts a normal full-disc frame and the bundled cropped skin", () => {
    expect(isUsableSunFrame(0.25)).toBe(true); // full disc on black background
    expect(isUsableSunFrame(0.5)).toBe(true); // tight-cropped bundled skin
  });

  it("treats the threshold itself as usable", () => {
    expect(isUsableSunFrame(SUN_FRAME_MIN_LUMA)).toBe(true);
    expect(isUsableSunFrame(SUN_FRAME_MIN_LUMA - 1e-6)).toBe(false);
  });
});
