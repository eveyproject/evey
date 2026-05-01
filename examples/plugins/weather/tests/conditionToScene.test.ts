// Tests for conditionToScene — covers all major WMO code bands.
import { describe, it, expect } from "vitest";
import { conditionToScene } from "../src/domain/mapping/conditionToScene";
import type { WeatherSnapshot } from "../src/domain/weather";

function snap(code: number, isDay = true): WeatherSnapshot {
  return {
    conditionCode: code,
    cloudCoverPct: 0,
    precipitationType: "none",
    precipIntensity: 0,
    hasThunder: false,
    temperatureC: 20,
    temperatureDisplay: 20,
    windKph: 10,
    isDay,
    units: "celsius",
    cityName: "TestCity",
  };
}

const NOW = new Date("2024-06-15T12:00:00"); // noon

describe("conditionToScene", () => {
  it("code 0 (clear day): cloudCover=0, precipitation=none, no thunder", () => {
    const p = conditionToScene(snap(0, true), NOW);
    expect(p.cloudCover).toBe(0);
    expect(p.cloudDensity).toBe(0);
    expect(p.precipitation).toBe("none");
    expect(p.thunderRate).toBe(0);
    expect(p.fog).toBe(0);
    expect(p.isDay).toBe(true);
    expect(p.label).toBe("Clear");
    expect(p.labelKey).toBe("clear_day");
  });

  it("code 0 (clear night): label reflects night", () => {
    const p = conditionToScene(snap(0, false), NOW);
    expect(p.cloudCover).toBe(0);
    expect(p.isDay).toBe(false);
    expect(p.label).toBe("Clear night");
    expect(p.labelKey).toBe("clear_night");
  });

  it("code 1 (mainly clear): low cloud cover", () => {
    const p = conditionToScene(snap(1), NOW);
    expect(p.cloudCover).toBeGreaterThan(0);
    expect(p.cloudCover).toBeLessThan(0.3);
    expect(p.precipitation).toBe("none");
    expect(p.thunderRate).toBe(0);
  });

  it("code 2 (partly cloudy): medium cloud cover", () => {
    const p = conditionToScene(snap(2), NOW);
    expect(p.cloudCover).toBeGreaterThanOrEqual(0.3);
    expect(p.cloudCover).toBeLessThan(0.7);
    expect(p.precipitation).toBe("none");
    expect(p.labelKey).toBe("partly_cloudy");
  });

  it("code 3 (overcast): high cloud cover", () => {
    const p = conditionToScene(snap(3), NOW);
    expect(p.cloudCover).toBeGreaterThanOrEqual(0.8);
    expect(p.precipitation).toBe("none");
    expect(p.thunderRate).toBe(0);
  });

  it("code 45 (fog): very high cloud cover, high fog", () => {
    const p = conditionToScene(snap(45), NOW);
    expect(p.cloudCover).toBeGreaterThanOrEqual(0.85);
    expect(p.fog).toBeGreaterThan(0.5);
    expect(p.precipitation).toBe("none");
  });

  it("code 48 (rime fog): fog > 0.5", () => {
    const p = conditionToScene(snap(48), NOW);
    expect(p.fog).toBeGreaterThan(0.5);
  });

  it("code 61 (light rain): precipitation=rain, precipIntensity in 0..1", () => {
    const p = conditionToScene(snap(61), NOW);
    expect(p.precipitation).toBe("rain");
    expect(p.precipIntensity).toBeGreaterThan(0);
    expect(p.precipIntensity).toBeLessThanOrEqual(1);
    expect(p.cloudCover).toBeGreaterThan(0.7);
    expect(p.thunderRate).toBe(0);
  });

  it("code 65 (heavy rain): high precipIntensity", () => {
    const p = conditionToScene(snap(65), NOW);
    expect(p.precipitation).toBe("rain");
    expect(p.precipIntensity).toBeGreaterThan(0.7);
  });

  it("code 71 (light snow): precipitation=snow", () => {
    const p = conditionToScene(snap(71), NOW);
    expect(p.precipitation).toBe("snow");
    expect(p.precipIntensity).toBeGreaterThan(0);
    expect(p.thunderRate).toBe(0);
  });

  it("code 75 (heavy snow): high precipIntensity", () => {
    const p = conditionToScene(snap(75), NOW);
    expect(p.precipitation).toBe("snow");
    expect(p.precipIntensity).toBeGreaterThan(0.6);
  });

  it("code 80 (light showers): precipitation=rain", () => {
    const p = conditionToScene(snap(80), NOW);
    expect(p.precipitation).toBe("rain");
    expect(p.cloudCover).toBeGreaterThan(0.6);
  });

  it("code 85 (snow showers): precipitation=snow", () => {
    const p = conditionToScene(snap(85), NOW);
    expect(p.precipitation).toBe("snow");
  });

  it("code 86 (heavy snow showers): high precipIntensity", () => {
    const p = conditionToScene(snap(86), NOW);
    expect(p.precipitation).toBe("snow");
    expect(p.precipIntensity).toBeGreaterThan(0.5);
  });

  it("code 95 (thunderstorm): thunderRate > 0, precipitation=rain", () => {
    const p = conditionToScene(snap(95), NOW);
    expect(p.thunderRate).toBeGreaterThan(0);
    expect(p.precipitation).toBe("rain");
    expect(p.cloudCover).toBe(1.0);
  });

  it("code 96 (thunderstorm with hail): high thunderRate", () => {
    const p = conditionToScene(snap(96), NOW);
    expect(p.thunderRate).toBeGreaterThan(0.5);
    expect(p.precipitation).toBe("rain");
  });

  it("code 99 (violent thunderstorm with hail): thunderRate > 0.5", () => {
    const p = conditionToScene(snap(99), NOW);
    expect(p.thunderRate).toBeGreaterThan(0.5);
  });

  it("palette fields exist and are RGB triples", () => {
    const p = conditionToScene(snap(0), NOW);
    expect(p.palette.zenith).toHaveLength(3);
    expect(p.palette.horizon).toHaveLength(3);
    expect(p.palette.sun).toHaveLength(3);
    expect(p.palette.fog).toHaveLength(3);
  });

  it("windVec is always a normalized-ish 2-vector", () => {
    const p = conditionToScene(snap(2), NOW);
    const mag = Math.sqrt(p.windVec[0] ** 2 + p.windVec[1] ** 2);
    expect(mag).toBeCloseTo(1, 1);
  });

  it("unknown code falls back gracefully", () => {
    const p = conditionToScene(snap(42), NOW);
    expect(p.label).toBe("Unknown");
    expect(p.labelKey).toBe("unknown");
    expect(p.precipitation).toBe("none");
    expect(p.thunderRate).toBe(0);
  });
});
