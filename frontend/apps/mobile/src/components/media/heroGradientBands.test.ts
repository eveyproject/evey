import { describe, it, expect } from "vitest";
import { heroGradientBands } from "./heroGradientBands";

describe("heroGradientBands", () => {
  it("returns one opacity value per step", () => {
    expect(heroGradientBands(4)).toHaveLength(4);
  });

  it("reaches full opacity on the last (bottom-most) band", () => {
    const bands = heroGradientBands(24);
    expect(bands[bands.length - 1]).toBe(1);
  });

  it("starts below full opacity on the first (top-most) band", () => {
    const bands = heroGradientBands(24);
    expect(bands[0]).toBeGreaterThan(0);
    expect(bands[0]).toBeLessThan(1);
  });

  it("is monotonically increasing - no band is darker than the one above it", () => {
    const bands = heroGradientBands(24);
    for (let i = 1; i < bands.length; i++) {
      expect(bands[i]).toBeGreaterThan(bands[i - 1]);
    }
  });

  it("keeps every value within the valid opacity range", () => {
    for (const b of heroGradientBands(24)) {
      expect(b).toBeGreaterThanOrEqual(0);
      expect(b).toBeLessThanOrEqual(1);
    }
  });

  it("returns an empty array for zero or negative steps", () => {
    expect(heroGradientBands(0)).toEqual([]);
    expect(heroGradientBands(-3)).toEqual([]);
  });
});
