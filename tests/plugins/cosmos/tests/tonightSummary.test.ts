import { describe, it, expect } from "vitest";
import { azToCompass, buildTonightSummary } from "../src/domain/services/tonightSummary";

describe("azToCompass", () => {
  it("maps the 8 cardinal/intercardinal directions", () => {
    expect(azToCompass(0)).toBe("N");
    expect(azToCompass(45)).toBe("NE");
    expect(azToCompass(90)).toBe("E");
    expect(azToCompass(135)).toBe("SE");
    expect(azToCompass(180)).toBe("S");
    expect(azToCompass(225)).toBe("SW");
    expect(azToCompass(270)).toBe("W");
    expect(azToCompass(315)).toBe("NW");
  });

  it("wraps past 360deg", () => {
    expect(azToCompass(359)).toBe("N");
  });
});

describe("buildTonightSummary", () => {
  it("picks the highest currently-visible planet", () => {
    const summary = buildTonightSummary("full", 180, [
      { name: "mars", altDeg: -5, azDeg: 10 },
      { name: "jupiter", altDeg: 40, azDeg: 135 },
      { name: "saturn", altDeg: 20, azDeg: 270 },
    ]);
    expect(summary.brightestVisiblePlanet).toEqual({ name: "jupiter", compass: "SE" });
    expect(summary.daysToFullMoon).toBeCloseTo(0, 6);
  });

  it("returns null when no planet is above the horizon", () => {
    const summary = buildTonightSummary("new", 0, [{ name: "mars", altDeg: -1, azDeg: 0 }]);
    expect(summary.brightestVisiblePlanet).toBeNull();
  });
});
