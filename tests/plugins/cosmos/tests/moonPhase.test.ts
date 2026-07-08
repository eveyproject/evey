import { describe, it, expect } from "vitest";
import { moonPhaseFromElongation, synodicAgeFraction, daysToFullMoon } from "../src/domain/services/moonPhase";
import { moonEclipticPosition } from "../src/domain/services/moonPosition";
import { sunPosition } from "../src/domain/services/sunPosition";

describe("moonPhaseFromElongation", () => {
  it("is 0% illuminated at elongation 0 (new moon) and 100% at 180 (full moon)", () => {
    expect(moonPhaseFromElongation(100, 100).illuminatedFraction).toBeCloseTo(0, 6);
    expect(moonPhaseFromElongation(280, 100).illuminatedFraction).toBeCloseTo(1, 6);
  });

  it("is ~50% illuminated at the quarters (elongation 90/270)", () => {
    expect(moonPhaseFromElongation(190, 100).illuminatedFraction).toBeCloseTo(0.5, 6);
    expect(moonPhaseFromElongation(10, 100).illuminatedFraction).toBeCloseTo(0.5, 6);
  });

  it("always returns a fraction in [0,1]", () => {
    for (let elong = 0; elong < 360; elong += 7) {
      const { illuminatedFraction } = moonPhaseFromElongation(elong, 0);
      expect(illuminatedFraction).toBeGreaterThanOrEqual(0);
      expect(illuminatedFraction).toBeLessThanOrEqual(1);
    }
  });
});

describe("cross-validation: elongation-based phase vs. the classic synodic-cycle estimate", () => {
  // Two independent models of the same physical quantity (illuminated
  // fraction) should broadly agree. The synodic model approximates
  // illuminated fraction as (1 - cos(2*pi*age))/2 where age is the fraction
  // of the ~29.53-day cycle elapsed since a known new moon.
  it.each([
    Date.UTC(2024, 0, 15, 6, 0, 0),
    Date.UTC(2024, 3, 3, 18, 0, 0),
    Date.UTC(2024, 6, 21, 9, 30, 0),
    Date.UTC(2024, 9, 10, 21, 0, 0),
    Date.UTC(2025, 1, 1, 0, 0, 0),
  ])("agrees within 0.15 illuminated fraction at %i", (ms) => {
    const moonLon = moonEclipticPosition(ms).lonDeg;
    const sunLon = sunPosition(ms).eclipticLonDeg;
    const real = moonPhaseFromElongation(moonLon, sunLon).illuminatedFraction;

    const age = synodicAgeFraction(ms);
    const approx = (1 - Math.cos(2 * Math.PI * age)) / 2;

    expect(Math.abs(real - approx)).toBeLessThan(0.15);
  });
});

describe("daysToFullMoon", () => {
  it("is 0 at full moon and ~14.77 at new moon", () => {
    expect(daysToFullMoon(180)).toBeCloseTo(0, 6);
    expect(daysToFullMoon(0)).toBeCloseTo(29.530588853 / 2, 2);
  });

  it("stays within [0, synodic period)", () => {
    for (let elong = 0; elong < 360; elong += 11) {
      const days = daysToFullMoon(elong);
      expect(days).toBeGreaterThanOrEqual(0);
      expect(days).toBeLessThan(29.53);
    }
  });
});
