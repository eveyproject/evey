// Greenwich and local mean sidereal time. Standard IAU low-precision formula
// (Meeus, "Astronomical Algorithms", eq. 12.4). The constant term
// (280.46061837 deg) is exactly the sidereal time at J2000.0 (2000-01-01
// 12:00 UT) - a well documented reference fact used as a regression check.

import { julianDate } from "./time";
import { normalizeDeg } from "./mathUtils";

const JD2000 = 2451545.0;

export function greenwichMeanSiderealTimeDeg(nowMs: number): number {
  const jd = julianDate(nowMs);
  const d = jd - JD2000;
  const T = d / 36525;
  const gmst =
    280.46061837 +
    360.98564736629 * d +
    0.000387933 * T * T -
    (T * T * T) / 38710000;
  return normalizeDeg(gmst);
}

// East longitude positive, west negative (matches GeoCoord.lon convention).
export function localSiderealTimeDeg(nowMs: number, lonDeg: number): number {
  return normalizeDeg(greenwichMeanSiderealTimeDeg(nowMs) + lonDeg);
}
