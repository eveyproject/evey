import { describe, it, expect } from "vitest";
import { conditionToScene } from "../src/domain/services/conditionToScene";
import type { WeatherSnapshot } from "../src/domain/model/weather";

function snap(code: number, isDay = true): WeatherSnapshot {
  return {
    conditionCode: code,
    precipitationType: "none",
    precipIntensity: 0,
    hasThunder: code >= 95,
    temperatureDisplay: 20,
    windKph: 10,
    isDay,
    units: "celsius",
    cityName: "Paris",
  };
}

describe("conditionToScene", () => {
  it("clear day", () => {
    const s = conditionToScene(snap(0, true));
    expect(s.cloudCover).toBe(0);
    expect(s.precipitation).toBe("none");
    expect(s.labelKey).toBe("clear_day");
    expect(s.mood).toBe("clear");
  });

  it("clear night switches label", () => {
    const s = conditionToScene(snap(0, false));
    expect(s.labelKey).toBe("clear_night");
    expect(s.label).toBe("Clear night");
  });

  it("code 1/2 stay clear-family regardless of night", () => {
    expect(conditionToScene(snap(1, false)).labelKey).toBe("mainly_clear");
    expect(conditionToScene(snap(2, false)).labelKey).toBe("partly_cloudy");
  });

  it("overcast", () => {
    const s = conditionToScene(snap(3));
    expect(s.cloudCover).toBeGreaterThan(0.8);
    expect(s.mood).toBe("cloudy");
  });

  it("fog", () => {
    const s = conditionToScene(snap(45));
    expect(s.fog).toBeGreaterThan(0.8);
    expect(s.mood).toBe("fog");
    expect(conditionToScene(snap(48)).labelKey).toBe("rime_fog");
  });

  it("rain intensities scale with code", () => {
    const light = conditionToScene(snap(61));
    const heavy = conditionToScene(snap(65));
    expect(light.precipitation).toBe("rain");
    expect(heavy.precipIntensity).toBeGreaterThan(light.precipIntensity);
    expect(heavy.mood).toBe("rain");
  });

  it("snow codes map to snow", () => {
    for (const code of [71, 73, 75, 77, 85, 86]) {
      expect(conditionToScene(snap(code)).precipitation).toBe("snow");
    }
  });

  it("thunderstorm has flashes and storm mood", () => {
    const s = conditionToScene(snap(95));
    expect(s.thunderRate).toBeGreaterThan(0);
    expect(s.mood).toBe("storm");
    expect(conditionToScene(snap(99)).thunderRate).toBeGreaterThan(s.thunderRate);
  });

  it("unknown code falls back", () => {
    const s = conditionToScene(snap(42));
    expect(s.labelKey).toBe("unknown");
  });

  it("every mapped labelKey is a non-empty string", () => {
    for (const code of [0, 1, 2, 3, 45, 48, 51, 53, 55, 56, 57, 61, 63, 65, 66, 67, 71, 73, 75, 77, 80, 81, 82, 85, 86, 95, 96, 99]) {
      const s = conditionToScene(snap(code));
      expect(s.labelKey.length).toBeGreaterThan(0);
      expect(s.cloudCover).toBeGreaterThanOrEqual(0);
      expect(s.cloudCover).toBeLessThanOrEqual(1);
      expect(s.precipIntensity).toBeGreaterThanOrEqual(0);
      expect(s.precipIntensity).toBeLessThanOrEqual(1);
    }
  });
});
