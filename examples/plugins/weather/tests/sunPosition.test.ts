// Sanity tests for sunPosition and moonPhase.
import { describe, it, expect } from "vitest";
import { sunPosition, moonPhase } from "../src/domain/mapping/sunPosition";

describe("sunPosition", () => {
  it("noon (hour=12): altitude is maximum (1.0)", () => {
    const noon = new Date("2024-06-15T12:00:00");
    const { altitude } = sunPosition(noon);
    // sin((12-6)/12 * π) = sin(π/2) = 1
    expect(altitude).toBeCloseTo(1.0, 4);
  });

  it("midnight (hour=0): altitude is minimum (negative)", () => {
    const midnight = new Date("2024-06-15T00:00:00");
    const { altitude } = sunPosition(midnight);
    // sin((0-6)/12 * π) = sin(-π/2) = -1
    expect(altitude).toBeCloseTo(-1.0, 4);
  });

  it("sunrise (hour=6): altitude ≈ 0", () => {
    const sunrise = new Date("2024-06-15T06:00:00");
    const { altitude } = sunPosition(sunrise);
    expect(altitude).toBeCloseTo(0, 4);
  });

  it("morning (hour=9): altitude positive but < noon", () => {
    const morning = new Date("2024-06-15T09:00:00");
    const noon    = new Date("2024-06-15T12:00:00");
    const { altitude: altMorning } = sunPosition(morning);
    const { altitude: altNoon }    = sunPosition(noon);
    expect(altMorning).toBeGreaterThan(0);
    expect(altMorning).toBeLessThan(altNoon);
  });

  it("evening (hour=18): altitude negative (sun setting)", () => {
    const evening = new Date("2024-06-15T21:00:00");
    const { altitude } = sunPosition(evening);
    expect(altitude).toBeLessThan(0);
  });

  it("azimuth is in -π..π range", () => {
    for (let h = 0; h < 24; h++) {
      const d = new Date(`2024-06-15T${String(h).padStart(2, "0")}:00:00`);
      const { azimuth } = sunPosition(d);
      expect(azimuth).toBeGreaterThanOrEqual(-Math.PI);
      expect(azimuth).toBeLessThanOrEqual(Math.PI);
    }
  });
});

describe("moonPhase", () => {
  it("returns value in 0..1", () => {
    const phase = moonPhase(new Date("2024-06-15T00:00:00"));
    expect(phase).toBeGreaterThanOrEqual(0);
    expect(phase).toBeLessThan(1);
  });

  it("reference new moon date gives phase ≈ 0", () => {
    const refNewMoon = new Date("2000-01-06T18:14:00Z");
    const phase = moonPhase(refNewMoon);
    expect(phase).toBeCloseTo(0, 3);
  });

  it("half synodic period away gives phase ≈ 0.5 (full moon)", () => {
    const refNewMoon = new Date("2000-01-06T18:14:00Z");
    const synodicMs  = 29.530588853 * 24 * 3600 * 1000;
    const fullMoon   = new Date(refNewMoon.getTime() + synodicMs / 2);
    const phase      = moonPhase(fullMoon);
    expect(phase).toBeCloseTo(0.5, 2);
  });
});
