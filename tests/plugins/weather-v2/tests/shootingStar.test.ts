import { describe, it, expect } from "vitest";
import { initShootingStar, shootingStar, type ShootingStar } from "../src/domain/services/shootingStar";

// Production calls shootingStar(out, ...) and mutates a single long-lived
// `out` object (zero allocation in the render loop, see GlRenderer.ts). Tests
// use a throwaway target per sample - allocation-per-assertion is fine here,
// only the production hot path must stay allocation-free.
function sample(timeSec: number, seed: number, active: boolean): ShootingStar {
  const out = initShootingStar();
  shootingStar(out, timeSec, seed, active);
  return out;
}

describe("shootingStar", () => {
  it("inactive gate always hides the star, regardless of time/seed", () => {
    for (const t of [0, 12.5, 500, 100000]) {
      const s = sample(t, 42, false);
      expect(s.visible).toBe(false);
      expect(s.life).toBe(0);
    }
  });

  it("negative or non-finite time hides the star instead of throwing", () => {
    expect(sample(-1, 0, true).visible).toBe(false);
    expect(sample(NaN, 0, true).visible).toBe(false);
    expect(sample(Infinity, 0, true).visible).toBe(false);
  });

  it("mutates the target object in place instead of allocating a new one", () => {
    const out = initShootingStar();
    const before = out;
    shootingStar(out, 133.7, 7, true);
    expect(out).toBe(before); // same reference - zero allocation in the hot path
  });

  it("is a deterministic pure function of (time, seed, active)", () => {
    const a = sample(133.7, 7, true);
    const b = sample(133.7, 7, true);
    expect(a).toEqual(b);
  });

  it("different seeds decorrelate the schedule", () => {
    // Sample many time points; the two seeds should not agree on every
    // visible/hidden flag (they would if the seed had no effect).
    let disagreements = 0;
    for (let t = 0; t < 2000; t += 0.5) {
      if (sample(t, 1, true).visible !== sample(t, 2, true).visible) disagreements++;
    }
    expect(disagreements).toBeGreaterThan(0);
  });

  it("is rare: visible only a small fraction of the time", () => {
    let visibleSamples = 0;
    let total = 0;
    for (let t = 0; t < 5000; t += 0.2) {
      total++;
      if (sample(t, 99, true).visible) visibleSamples++;
    }
    const fraction = visibleSamples / total;
    expect(fraction).toBeGreaterThan(0); // it does happen sometimes
    expect(fraction).toBeLessThan(0.1); // but rarely, never a constant overlay
  });

  it("while visible, all fields stay finite and life fades toward the end of the streak", () => {
    let found = false;
    for (let t = 0; t < 5000 && !found; t += 0.05) {
      const s = sample(t, 3, true);
      if (!s.visible) continue;
      found = true;
      for (const v of [s.x, s.y, s.dirX, s.dirY, s.life]) expect(Number.isFinite(v)).toBe(true);
      expect(s.life).toBeGreaterThan(0);
      expect(s.life).toBeLessThanOrEqual(1);
      expect(Math.hypot(s.dirX, s.dirY)).toBeCloseTo(1, 5);
    }
    expect(found).toBe(true);
  });
});
