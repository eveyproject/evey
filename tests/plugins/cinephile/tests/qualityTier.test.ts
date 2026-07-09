import { describe, it, expect } from "vitest";
import { maxNodesForTier, glowEnabledForTier } from "../src/domain/services/qualityTier";

describe("maxNodesForTier", () => {
  it("increases capacity with tier", () => {
    expect(maxNodesForTier("low")).toBeLessThan(maxNodesForTier("mid"));
    expect(maxNodesForTier("mid")).toBeLessThan(maxNodesForTier("high"));
  });

  it("returns positive integers for every tier", () => {
    for (const tier of ["low", "mid", "high"] as const) {
      expect(Number.isInteger(maxNodesForTier(tier))).toBe(true);
      expect(maxNodesForTier(tier)).toBeGreaterThan(0);
    }
  });
});

describe("glowEnabledForTier", () => {
  it("disables glow on low tier only", () => {
    expect(glowEnabledForTier("low")).toBe(false);
    expect(glowEnabledForTier("mid")).toBe(true);
    expect(glowEnabledForTier("high")).toBe(true);
  });
});
