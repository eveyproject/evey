import { describe, expect, it } from "vitest";
import { DEFAULT_UI_SCALE, UI_SCALE_OPTIONS, effectiveUiScale, isUiScaleOption } from "./uiScale";

// The vocabulary itself. That the default matches the Android plugin's own
// defaultPercent is checked where app.json lives (apps/tv/src/lib).
describe("uiScale", () => {
  it("holds a stored value that is no longer offered, so the tick never lies", () => {
    expect(effectiveUiScale(85)).toBe(85);
    expect(isUiScaleOption(85)).toBe(false);
    expect(isUiScaleOption(UI_SCALE_OPTIONS[0])).toBe(true);
  });

  it("falls back to the default for anything unusable", () => {
    expect(effectiveUiScale(null)).toBe(DEFAULT_UI_SCALE);
    expect(effectiveUiScale(undefined)).toBe(DEFAULT_UI_SCALE);
    expect(effectiveUiScale(0)).toBe(DEFAULT_UI_SCALE);
    expect(effectiveUiScale(Number.NaN)).toBe(DEFAULT_UI_SCALE);
  });
});
