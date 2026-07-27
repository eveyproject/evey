import { describe, it, expect } from "vitest";
import { defaultTheme, lightDefaultTheme } from "@evey/theme";
import { getChartPalette, chartColorAt } from "./chartPalette";

describe("chartPalette", () => {
  it("returns 8 distinct colors", () => {
    const palette = getChartPalette(defaultTheme);
    expect(palette).toHaveLength(8);
    expect(new Set(palette).size).toBe(8);
  });

  it("stays distinct on a light theme too", () => {
    const palette = getChartPalette(lightDefaultTheme);
    expect(new Set(palette).size).toBe(8);
  });

  it("is stable for a given index across repeated calls (no reshuffling on re-render)", () => {
    const first = chartColorAt(defaultTheme, 3);
    const second = chartColorAt(defaultTheme, 3);
    expect(first).toBe(second);
  });

  it("wraps around when there are more series than palette slots", () => {
    const palette = getChartPalette(defaultTheme);
    expect(chartColorAt(defaultTheme, 8)).toBe(palette[0]);
    expect(chartColorAt(defaultTheme, 9)).toBe(palette[1]);
  });

  it("leads with the theme's own accent color", () => {
    expect(getChartPalette(defaultTheme)[0]).toBe(defaultTheme.colors.accent);
  });
});
