// Geocentric position of the Sun from the SUN_ELEMENTS row, which is really
// Earth's own heliocentric orbit: since N=i=0 for that row, the "heliocentric"
// x/y the two-body solver returns for it IS the Sun's apparent geocentric
// ecliptic position (Earth-relative), with latitude exactly 0 (the Sun is
// always on the ecliptic by definition).

import { heliocentricPosition, SUN_ELEMENTS } from "./orbitalElements";
import { eclipticToEquatorial, type EquatorialCoord } from "./coordinates";
import { daysSinceElementsEpoch, meanObliquityDeg } from "./time";

export interface SunPosition extends EquatorialCoord {
  eclipticLonDeg: number;
  distanceAu: number;
}

export function sunPosition(nowMs: number): SunPosition {
  const d = daysSinceElementsEpoch(nowMs);
  const helio = heliocentricPosition(SUN_ELEMENTS, d);
  const equatorial = eclipticToEquatorial(helio.trueLonDeg, 0, meanObliquityDeg(nowMs));
  return { ...equatorial, eclipticLonDeg: helio.trueLonDeg, distanceAu: helio.distanceAu };
}

// Sun geocentric ecliptic rectangular coordinates (z=0), used by
// planetPositions.ts to go from heliocentric to geocentric.
export function sunGeocentricRectangular(nowMs: number): { xs: number; ys: number } {
  const d = daysSinceElementsEpoch(nowMs);
  const helio = heliocentricPosition(SUN_ELEMENTS, d);
  return { xs: helio.xh, ys: helio.yh };
}
