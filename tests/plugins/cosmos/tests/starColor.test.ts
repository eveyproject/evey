import { describe, it, expect } from "vitest";
import { bvToRgb } from "../src/domain/services/starColor";

describe("bvToRgb", () => {
  it("makes hot (negative B-V) stars bluer than cool (high B-V) stars", () => {
    const hot = bvToRgb(-0.3);
    const cool = bvToRgb(1.8);
    expect(hot[2] / hot[0]).toBeGreaterThan(cool[2] / cool[0]);
  });

  it("returns near-neutral white around B-V ~0.4 (solar-neighborhood F/G stars)", () => {
    const [r, g, b] = bvToRgb(0.4);
    expect(Math.max(r, g, b) - Math.min(r, g, b)).toBeLessThan(0.1);
  });

  it("clamps outside the defined range instead of extrapolating wildly", () => {
    const veryBlue = bvToRgb(-5);
    const veryRed = bvToRgb(5);
    for (const c of [...veryBlue, ...veryRed]) {
      expect(c).toBeGreaterThanOrEqual(0);
      expect(c).toBeLessThanOrEqual(1);
    }
  });
});
