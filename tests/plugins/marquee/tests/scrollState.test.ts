import { describe, it, expect } from "vitest";
import { advanceScroll } from "../src/domain/services/scrollState";

describe("advanceScroll", () => {
  it("advances by speed * dt when within bounds", () => {
    expect(advanceScroll(0, 1, 2, 100)).toBe(2);
  });

  it("wraps forward past the period", () => {
    expect(advanceScroll(99, 1, 2, 100)).toBe(1);
  });

  it("wraps backward for negative speed", () => {
    expect(advanceScroll(1, 1, -2, 100)).toBe(99);
  });

  it("always stays within [0, period)", () => {
    let offset = 0;
    for (let i = 0; i < 1000; i++) {
      offset = advanceScroll(offset, 0.37, 5, 13);
      expect(offset).toBeGreaterThanOrEqual(0);
      expect(offset).toBeLessThan(13);
    }
  });

  it("returns 0 for a degenerate (non-positive) period", () => {
    expect(advanceScroll(5, 1, 2, 0)).toBe(0);
    expect(advanceScroll(5, 1, 2, -10)).toBe(0);
  });

  it("is a no-op when speed is 0", () => {
    expect(advanceScroll(42, 1, 0, 100)).toBe(42);
  });
});
