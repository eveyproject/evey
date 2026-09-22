import { describe, it, expect } from "vitest";
import { DEFAULT_UI_SCALE, UI_SCALE_OPTIONS, effectiveUiScale, isUiScaleOption } from "./uiScale";
import appJson from "../../app.json";

describe("uiScale", () => {
  it("the default matches the native plugin's defaultPercent (one number, two sides)", () => {
    const plugin = (appJson.expo.plugins as unknown[]).find((p) => Array.isArray(p) && p[0] === "./plugins/withUiScale") as [string, { defaultPercent: number }];
    expect(plugin[1].defaultPercent).toBe(DEFAULT_UI_SCALE);
    expect(isUiScaleOption(DEFAULT_UI_SCALE)).toBe(true);
  });

  it("never set is the default; a stored value is itself, even outside the picker", () => {
    expect(effectiveUiScale(null)).toBe(DEFAULT_UI_SCALE);
    expect(effectiveUiScale(undefined)).toBe(DEFAULT_UI_SCALE);
    expect(effectiveUiScale(0)).toBe(DEFAULT_UI_SCALE);
    expect(effectiveUiScale(NaN)).toBe(DEFAULT_UI_SCALE);
    expect(effectiveUiScale(90)).toBe(90);
    expect(effectiveUiScale(85.4)).toBe(85);
    expect(isUiScaleOption(85)).toBe(false);
    expect(UI_SCALE_OPTIONS).toEqual([70, 80, 90, 100]);
  });
});
