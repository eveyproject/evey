import { describe, it, expect } from "vitest";
import { backdropWidthForViewport } from "./backdropWidth";

describe("backdropWidthForViewport", () => {
  it("multiplies viewport width by device pixel ratio", () => {
    expect(backdropWidthForViewport(390, 3)).toBe(1170);
  });

  it("rounds a fractional result", () => {
    expect(backdropWidthForViewport(412, 2.625)).toBe(1082);
  });

  it("handles a pixel ratio of 1 (low-density devices)", () => {
    expect(backdropWidthForViewport(800, 1)).toBe(800);
  });

  it("returns 0 for a zero viewport width so callers fall back to full-size", () => {
    expect(backdropWidthForViewport(0, 3)).toBe(0);
  });

  it("returns 0 for a negative or NaN input instead of a bogus width", () => {
    expect(backdropWidthForViewport(-100, 3)).toBe(0);
    expect(backdropWidthForViewport(NaN, 3)).toBe(0);
  });
});
