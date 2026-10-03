import { describe, expect, it, vi } from "vitest";

vi.mock("react-native", () => ({ Dimensions: { get: () => ({ width: 411, height: 900 }) } }));

import { posterGrid } from "./posterGrid";

describe("posterGrid", () => {
  it("keeps a phone at 3 columns with the same cell width", () => {
    const g = posterGrid(411);
    expect(g.columns).toBe(3);
    expect(g.itemWidth).toBeCloseTo((411 - 8 * 4) / 3);
  });

  it("adds columns on a 914 dp tablet and keeps posters near 170 dp", () => {
    const g = posterGrid(914);
    expect(g.columns).toBe(5);
    expect(g.itemWidth).toBeGreaterThan(150);
    expect(g.itemWidth).toBeLessThan(190);
  });

  it("never overflows the content width beside the rail", () => {
    for (const w of [600, 768, 914, 1280]) {
      const g = posterGrid(w);
      expect(g.columns * (g.itemWidth + 8) + 8).toBeLessThanOrEqual(w - 52 + 0.001);
    }
  });

  it("honours a 2-column phone floor with a 16 dp gap (music)", () => {
    const phone = posterGrid(411, 16, 2);
    expect(phone.columns).toBe(2);
    expect(phone.itemWidth).toBeCloseTo((411 - 16 * 3) / 2);
    const tablet = posterGrid(1280, 16, 2);
    expect(tablet.columns).toBeGreaterThan(4);
    expect(tablet.itemWidth).toBeLessThan(200);
  });
});
