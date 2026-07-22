import { describe, it, expect } from "vitest";
import * as fs from "fs";
import * as path from "path";
import { fileURLToPath } from "url";
import { encode } from "jpeg-js";
// Backend-side guard (CJS, shipped next to main.js).
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore - plain JS module without type declarations
import { sunFrameLuma, isUsableSunFrame, SUN_FRAME_MIN_LUMA, prepareSunFrame } from "../sunFrameGuard.js";
import { decode } from "jpeg-js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// Encode a solid-color JPEG the way SDO frames are shaped (square RGBA).
function solidJpeg(r: number, g: number, b: number, size = 64): Uint8Array {
  const data = new Uint8Array(size * size * 4);
  for (let i = 0; i < size * size; i++) {
    data[i * 4] = r;
    data[i * 4 + 1] = g;
    data[i * 4 + 2] = b;
    data[i * 4 + 3] = 255;
  }
  return new Uint8Array(encode({ data, width: size, height: size }, 90).data);
}

describe("sunFrameLuma", () => {
  it("rejects the REAL dark frame NASA served on 2026-07-22", () => {
    // Ground truth: the exact latest_1024_0171.jpg that painted the sun black.
    // A valid JPEG (passes the SOI check) whose only bright pixels are the
    // caption text - it must never be persisted as sun-live.
    const buf = fs.readFileSync(path.join(__dirname, "fixtures", "sdo_dark_frame_2026_07_22.jpg"));
    const luma = sunFrameLuma(buf);
    expect(luma).not.toBeNull();
    expect(luma!).toBeLessThan(SUN_FRAME_MIN_LUMA);
    expect(isUsableSunFrame(luma)).toBe(false);
  });

  it("accepts a bright golden disc", () => {
    const luma = sunFrameLuma(solidJpeg(230, 160, 40));
    expect(luma).not.toBeNull();
    expect(isUsableSunFrame(luma)).toBe(true);
  });

  it("rejects an all-black frame", () => {
    expect(isUsableSunFrame(sunFrameLuma(solidJpeg(0, 0, 0)))).toBe(false);
  });

  it("returns null (rejected) for an undecodable buffer", () => {
    const luma = sunFrameLuma(new Uint8Array([0xff, 0xd8, 0x00, 0x01, 0x02]));
    expect(luma).toBeNull();
    expect(isUsableSunFrame(luma)).toBe(false);
  });

  it("keeps the same threshold as the frontend domain gate", async () => {
    const front = await import("../src/domain/services/sunImage");
    expect(SUN_FRAME_MIN_LUMA).toBe(front.SUN_FRAME_MIN_LUMA);
  });
});

// Mean ring luminance at radius fraction f of half-width (the same probe used
// to calibrate the crop: the AIA limb shows as the brightness peak).
function ringLuma(rgba: { width: number; height: number; data: Uint8Array }, f: number): number {
  const { width: W, height: H, data } = rgba;
  const cx = W / 2, cy = H / 2;
  let s = 0;
  const N = 128;
  for (let a = 0; a < N; a++) {
    const th = (a / N) * 2 * Math.PI;
    const x = Math.min(W - 1, Math.max(0, Math.round(cx + Math.cos(th) * f * (W / 2))));
    const y = Math.min(H - 1, Math.max(0, Math.round(cy + Math.sin(th) * f * (H / 2))));
    const o = (y * W + x) * 4;
    s += 0.2126 * data[o] + 0.7152 * data[o + 1] + 0.0722 * data[o + 2];
  }
  return s / N;
}

function limbFraction(rgba: { width: number; height: number; data: Uint8Array }): number {
  let max = 0;
  const samples: Array<[number, number]> = [];
  for (let f = 0.55; f <= 1.0; f += 0.01) {
    const v = ringLuma(rgba, f);
    samples.push([f, v]);
    if (v > max) max = v;
  }
  let limb = 0.55;
  for (const [f, v] of samples) if (v >= max * 0.95) limb = f;
  return limb;
}

describe("prepareSunFrame", () => {
  it("normalizes the REAL full-disc frame to the bundled-skin geometry", () => {
    // SDO full-disc frames keep their limb at ~0.78 of the half-width; the
    // shader expects the bundled tight crop (~0.93), and the mismatch showed
    // as a nibbled disc edge (reported 2026-07-22). The prepared frame must
    // land within the shader's tolerance band.
    const buf = fs.readFileSync(path.join(__dirname, "fixtures", "sdo_full_disc_2026_07_22.jpg"));
    const res = prepareSunFrame(buf);
    expect(res.ok).toBe(true);
    const out = decode(res.jpeg, { useTArray: true, maxMemoryUsageInMB: 64 });
    expect(out.width).toBe(384);
    expect(out.height).toBe(384);
    const limb = limbFraction(out as never);
    expect(limb).toBeGreaterThanOrEqual(0.88);
    expect(limb).toBeLessThanOrEqual(0.98);
  });

  it("leaves an already-tight synthetic disc essentially unchanged", () => {
    // Solid disc with its limb already at 0.92: the crop factor should be ~1,
    // i.e. no double-zoom on a frame that is already bundled-skin shaped.
    const size = 256;
    const data = new Uint8Array(size * size * 4);
    const r = 0.92 * (size / 2);
    for (let y = 0; y < size; y++) {
      for (let x = 0; x < size; x++) {
        const inside = Math.hypot(x - size / 2, y - size / 2) <= r;
        const o = (y * size + x) * 4;
        data[o] = inside ? 220 : 0;
        data[o + 1] = inside ? 150 : 0;
        data[o + 2] = inside ? 40 : 0;
        data[o + 3] = 255;
      }
    }
    const buf = new Uint8Array(encode({ data, width: size, height: size }, 90).data);
    const res = prepareSunFrame(buf);
    expect(res.ok).toBe(true);
    const out = decode(res.jpeg, { useTArray: true, maxMemoryUsageInMB: 64 });
    const limb = limbFraction(out as never);
    expect(limb).toBeGreaterThanOrEqual(0.88);
    expect(limb).toBeLessThanOrEqual(0.98);
  });

  it("still rejects the dark frame", () => {
    const buf = fs.readFileSync(path.join(__dirname, "fixtures", "sdo_dark_frame_2026_07_22.jpg"));
    const res = prepareSunFrame(buf);
    expect(res.ok).toBe(false);
    expect(res.luma).toBeLessThan(SUN_FRAME_MIN_LUMA);
  });

  it("rejects an undecodable buffer", () => {
    expect(prepareSunFrame(new Uint8Array([0xff, 0xd8, 0x00])).ok).toBe(false);
  });
});
