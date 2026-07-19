import { describe, it, expect } from "vitest";
import { CHART_COLORS } from "./chartColors";

describe("CHART_COLORS", () => {
  it("routes every series colour through a themeable CSS variable", () => {
    // A categorical palette still needs controlled hue separation, so the hex
    // literals stay as fallbacks - but a theme must be able to override each
    // slot. Seven of the eight used to be bare hex, which no theme could reach.
    for (const c of CHART_COLORS) {
      expect(c).toMatch(/^var\(--[a-z0-9-]+, *#[0-9a-f]{6}\)$/i);
    }
  });

  it("gives each series its own variable", () => {
    const names = CHART_COLORS.map((c) => c.slice(4, c.indexOf(",")).trim());
    expect(new Set(names).size).toBe(CHART_COLORS.length);
  });
});
