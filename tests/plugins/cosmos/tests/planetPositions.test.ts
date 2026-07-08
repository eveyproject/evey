import { describe, it, expect } from "vitest";
import { planetPosition, elongationDeg } from "../src/domain/services/planetPositions";
import { sunPosition } from "../src/domain/services/sunPosition";

// Real, well-documented orbital facts used as regression bounds (heliocentric
// perihelion/aphelion distances in AU):
//   Venus:   0.718 - 0.728     Mars:    1.381 - 1.666
//   Jupiter: 4.951 - 5.455     Saturn:  9.041 - 10.124
// And: Venus's greatest elongation from the Sun never exceeds ~48deg (a
// textbook fact - it is an inferior planet, always seen near the Sun).
function sampleDates(): number[] {
  const dates: number[] = [];
  for (let year = 2023; year <= 2026; year++) {
    for (let month = 0; month < 12; month += 2) {
      dates.push(Date.UTC(year, month, 10, 0, 0, 0));
    }
  }
  return dates;
}

describe("planetPosition - heliocentric distance stays within the real orbital range", () => {
  it.each(sampleDates())("venus at %i", (ms) => {
    const { heliocentricDistanceAu } = planetPosition("venus", ms);
    expect(heliocentricDistanceAu).toBeGreaterThan(0.715);
    expect(heliocentricDistanceAu).toBeLessThan(0.73);
  });

  it.each(sampleDates())("mars at %i", (ms) => {
    const { heliocentricDistanceAu } = planetPosition("mars", ms);
    expect(heliocentricDistanceAu).toBeGreaterThan(1.38);
    expect(heliocentricDistanceAu).toBeLessThan(1.67);
  });

  it.each(sampleDates())("jupiter at %i", (ms) => {
    const { heliocentricDistanceAu } = planetPosition("jupiter", ms);
    expect(heliocentricDistanceAu).toBeGreaterThan(4.95);
    expect(heliocentricDistanceAu).toBeLessThan(5.46);
  });

  it.each(sampleDates())("saturn at %i", (ms) => {
    const { heliocentricDistanceAu } = planetPosition("saturn", ms);
    expect(heliocentricDistanceAu).toBeGreaterThan(9.0);
    expect(heliocentricDistanceAu).toBeLessThan(10.13);
  });
});

describe("planetPosition - Venus never strays far from the Sun (inferior planet)", () => {
  it.each(sampleDates())("elongation stays under 48deg at %i", (ms) => {
    const venus = planetPosition("venus", ms);
    const sun = sunPosition(ms);
    const elong = elongationDeg(venus.eclipticLonDeg, sun.eclipticLonDeg);
    expect(elong).toBeLessThan(48);
  });
});
