import { describe, it, expect } from "vitest";
import { LABELS, translate } from "../src/i18n/labels";
import { conditionToScene } from "../src/domain/services/conditionToScene";
import type { WeatherSnapshot } from "../src/domain/model/weather";

describe("labels", () => {
  it("every key has all four locales, non-empty", () => {
    for (const [key, dict] of Object.entries(LABELS)) {
      for (const loc of ["en", "fr", "es", "de"] as const) {
        expect(dict[loc], `${key}.${loc}`).toBeTruthy();
      }
    }
  });

  it("translate falls back to en, then key", () => {
    expect(translate("rain", "fr")).toBe("Pluie modérée");
    expect(translate("rain", "pt")).toBe("Moderate rain");
    expect(translate("rain", undefined)).toBe("Moderate rain");
    expect(translate("does_not_exist", "fr")).toBe("does_not_exist");
  });

  it("locale region variants resolve (fr-FR)", () => {
    expect(translate("snow", "fr-FR")).toBe("Neige modérée");
  });

  it("every labelKey emitted by conditionToScene exists in LABELS", () => {
    const snap = (code: number, isDay: boolean): WeatherSnapshot => ({
      conditionCode: code,
      precipitationType: "none",
      precipIntensity: 0,
      hasThunder: false,
      temperatureDisplay: 0,
      windKph: 0,
      isDay,
      units: "celsius",
      cityName: "x",
    });
    for (let code = 0; code <= 99; code++) {
      for (const isDay of [true, false]) {
        const key = conditionToScene(snap(code, isDay)).labelKey;
        expect(LABELS[key], `labelKey ${key} (code ${code})`).toBeDefined();
      }
    }
  });
});
