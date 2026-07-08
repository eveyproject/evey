import { describe, it, expect } from "vitest";
import { shootingStar } from "../src/domain/services/shootingStar";

describe("shootingStar", () => {
  it("inactive gate always hides the star, regardless of time/seed", () => {
    for (const t of [0, 12.5, 500, 100000]) {
      const s = shootingStar(t, 42, false);
      expect(s.visible).toBe(false);
      expect(s.life).toBe(0);
    }
  });

  it("negative or non-finite time hides the star instead of throwing", () => {
    expect(shootingStar(-1, 0, true).visible).toBe(false);
    expect(shootingStar(NaN, 0, true).visible).toBe(false);
    expect(shootingStar(Infinity, 0, true).visible).toBe(false);
  });

  it("is a deterministic pure function of (time, seed, active)", () => {
    const a = shootingStar(133.7, 7, true);
    const b = shootingStar(133.7, 7, true);
    expect(a).toEqual(b);
  });

  it("different seeds decorrelate the schedule", () => {
    // Sample many time points; the two seeds should not agree on every
    // visible/hidden flag (they would if the seed had no effect).
    let disagreements = 0;
    for (let t = 0; t < 2000; t += 0.5) {
      if (shootingStar(t, 1, true).visible !== shootingStar(t, 2, true).visible) disagreements++;
    }
    expect(disagreements).toBeGreaterThan(0);
  });

  it("is rare: visible only a small fraction of the time", () => {
    let visibleSamples = 0;
    let total = 0;
    for (let t = 0; t < 5000; t += 0.2) {
      total++;
      if (shootingStar(t, 99, true).visible) visibleSamples++;
    }
    const fraction = visibleSamples / total;
    expect(fraction).toBeGreaterThan(0); // it does happen sometimes
    expect(fraction).toBeLessThan(0.1); // but rarely, never a constant overlay
  });

  it("while visible, all fields stay finite and life fades toward the end of the streak", () => {
    let found = false;
    for (let t = 0; t < 5000 && !found; t += 0.05) {
      const s = shootingStar(t, 3, true);
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
