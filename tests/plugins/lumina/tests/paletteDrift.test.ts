import { describe, it, expect } from "vitest";
import { computePaletteFrame, PALETTE_CYCLE_MS } from "../src/domain/services/paletteDrift";
import { PRESETS, PRESET_IDS } from "../src/domain/services/presets";

function allFinite01(frame: ReturnType<typeof computePaletteFrame>): boolean {
  for (const c of [frame.veilA, frame.veilB, frame.veilC, frame.caustic, frame.vignette]) {
    for (const v of c) {
      if (!Number.isFinite(v) || v < 0 || v > 1) return false;
    }
  }
  return true;
}

describe("computePaletteFrame", () => {
  it("is a pure function: same inputs produce the exact same output", () => {
    const preset = PRESETS.aurore;
    const a = computePaletteFrame(preset, 123_456);
    const b = computePaletteFrame(preset, 123_456);
    expect(a).toEqual(b);
  });

  it("produces finite 0..1 channels across a full cycle, for every preset", () => {
    for (const id of PRESET_IDS) {
      const preset = PRESETS[id];
      for (let t = 0; t <= PALETTE_CYCLE_MS; t += PALETTE_CYCLE_MS / 37) {
        expect(allFinite01(computePaletteFrame(preset, t)), `${id} @ ${t}ms`).toBe(true);
      }
    }
  });

  it("loops back to the start after one full cycle", () => {
    const preset = PRESETS.ocean;
    const start = computePaletteFrame(preset, 0);
    const oneLoop = computePaletteFrame(preset, PALETTE_CYCLE_MS);
    const twoLoops = computePaletteFrame(preset, PALETTE_CYCLE_MS * 2);
    expect(oneLoop).toEqual(start);
    expect(twoLoops).toEqual(start);
  });

  it("veilA and veilB stay visibly out of phase (never equal) across a cycle", () => {
    const preset = PRESETS.neon;
    for (let t = 0; t < PALETTE_CYCLE_MS; t += PALETTE_CYCLE_MS / 23) {
      const frame = computePaletteFrame(preset, t);
      expect(frame.veilA).not.toEqual(frame.veilB);
    }
  });

  it("veilC (background wash) never exceeds the preset's brightest anchor luminance", () => {
    // veilC is a darkened blend of two anchors, and a lerp/smoothstep blend
    // of two anchors can never be brighter than the brighter of the two -
    // darkening it further only shrinks it more. Holds for every preset.
    const lum = (c: [number, number, number]) => c[0] * 0.299 + c[1] * 0.587 + c[2] * 0.114;
    for (const id of PRESET_IDS) {
      const preset = PRESETS[id];
      const maxAnchorLum = Math.max(...preset.anchors.map(lum));
      for (let t = 0; t < PALETTE_CYCLE_MS; t += PALETTE_CYCLE_MS / 17) {
        const frame = computePaletteFrame(preset, t);
        expect(lum(frame.veilC)).toBeLessThanOrEqual(maxAnchorLum + 1e-9);
      }
    }
  });

  it("caustic is a lift of veilA toward white (each channel >= veilA's)", () => {
    const preset = PRESETS.aurore;
    const frame = computePaletteFrame(preset, 42_000);
    for (let i = 0; i < 3; i++) {
      expect(frame.caustic[i]).toBeGreaterThanOrEqual(frame.veilA[i] - 1e-9);
    }
  });

  it("vignette is the preset's fixed accent, unaffected by elapsed time", () => {
    const preset = PRESETS.ocean;
    const a = computePaletteFrame(preset, 0);
    const b = computePaletteFrame(preset, PALETTE_CYCLE_MS * 3.7);
    expect(a.vignette).toEqual(preset.vignette);
    expect(b.vignette).toEqual(preset.vignette);
  });

  it("different presets produce different palettes at the same instant", () => {
    const a = computePaletteFrame(PRESETS.aurore, 10_000);
    const n = computePaletteFrame(PRESETS.neon, 10_000);
    expect(a.veilA).not.toEqual(n.veilA);
  });

  it("negative elapsed time still produces a valid, finite frame", () => {
    const frame = computePaletteFrame(PRESETS.aurore, -5_000);
    expect(allFinite01(frame)).toBe(true);
  });
});
