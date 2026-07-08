// Geocentric equatorial position of the naked-eye superior/inferior planets
// visible without a telescope: Venus, Mars, Jupiter, Saturn. Combines each
// planet's heliocentric position with the Sun's (== -Earth's heliocentric
// position) the same way Schlyter's method does: geocentric = heliocentric +
// (Sun's geocentric rectangular position).

import {
  heliocentricPosition,
  VENUS_ELEMENTS,
  MARS_ELEMENTS,
  JUPITER_ELEMENTS,
  SATURN_ELEMENTS,
  type OrbitalElements,
} from "./orbitalElements";
import { sunGeocentricRectangular } from "./sunPosition";
import { eclipticToEquatorial, type EquatorialCoord } from "./coordinates";
import { daysSinceElementsEpoch, meanObliquityDeg } from "./time";
import { toDeg, normalizeDeg } from "./mathUtils";

export type PlanetName = "venus" | "mars" | "jupiter" | "saturn";

const ELEMENTS: Record<PlanetName, OrbitalElements> = {
  venus: VENUS_ELEMENTS,
  mars: MARS_ELEMENTS,
  jupiter: JUPITER_ELEMENTS,
  saturn: SATURN_ELEMENTS,
};

export interface PlanetPosition extends EquatorialCoord {
  name: PlanetName;
  heliocentricDistanceAu: number;
  geocentricDistanceAu: number;
  eclipticLonDeg: number; // geocentric ecliptic longitude, used for elongation checks
}

export function planetPosition(name: PlanetName, nowMs: number): PlanetPosition {
  const d = daysSinceElementsEpoch(nowMs);
  const helio = heliocentricPosition(ELEMENTS[name], d);
  const { xs, ys } = sunGeocentricRectangular(nowMs);

  const xg = helio.xh + xs;
  const yg = helio.yh + ys;
  const zg = helio.zh;

  const lonDeg = normalizeDeg(toDeg(Math.atan2(yg, xg)));
  const latDeg = toDeg(Math.atan2(zg, Math.sqrt(xg * xg + yg * yg)));
  const geocentricDistanceAu = Math.sqrt(xg * xg + yg * yg + zg * zg);

  const equatorial = eclipticToEquatorial(lonDeg, latDeg, meanObliquityDeg(nowMs));

  return {
    ...equatorial,
    name,
    heliocentricDistanceAu: helio.distanceAu,
    geocentricDistanceAu,
    eclipticLonDeg: lonDeg,
  };
}

export function allPlanetPositions(nowMs: number): PlanetPosition[] {
  return (Object.keys(ELEMENTS) as PlanetName[]).map((name) => planetPosition(name, nowMs));
}

// Angular separation between two ecliptic longitudes, 0..180deg. Used for the
// "Venus never strays far from the Sun" elongation check.
export function elongationDeg(lonADeg: number, lonBDeg: number): number {
  const raw = Math.abs(normalizeDeg(lonADeg) - normalizeDeg(lonBDeg));
  return raw > 180 ? 360 - raw : raw;
}
