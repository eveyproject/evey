import { describe, it, expect } from "vitest";
import { spotlightPhase, spotlightX } from "../src/domain/services/spotlightSweep";

describe("spotlightPhase", () => {
  it("starts at 0", () => {
    expect(spotlightPhase(0, 1000)).toBe(0);
  });

  it("reaches its peak at half the period", () => {
    expect(spotlightPhase(500, 1000)).toBeCloseTo(1, 9);
  });

  it("returns to 0 at a full period (wraps)", () => {
    expect(spotlightPhase(1000, 1000)).toBeCloseTo(0, 9);
  });

  it("stays within [0,1]", () => {
    for (let t = 0; t < 5000; t += 37) {
      const p = spotlightPhase(t, 1000);
      expect(p).toBeGreaterThanOrEqual(0);
      expect(p).toBeLessThanOrEqual(1);
    }
  });

  it("returns 0 for a degenerate period", () => {
    expect(spotlightPhase(500, 0)).toBe(0);
  });
});

describe("spotlightX", () => {
  it("maps phase 0 to -halfWidth", () => {
    expect(spotlightX(0, 2)).toBe(-2);
  });

  it("maps phase 1 to +halfWidth", () => {
    expect(spotlightX(1, 2)).toBe(2);
  });

  it("maps phase 0.5 to the center", () => {
    expect(spotlightX(0.5, 2)).toBe(0);
  });
});
