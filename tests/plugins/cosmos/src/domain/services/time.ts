// Time conversions shared by every astronomical calculation in this plugin.
// Everything downstream (sidereal time, Sun/Moon/planet position) is derived
// from a single epoch day-count, so there is exactly one place that can get
// the calendar math wrong.

// Julian Date from a JS epoch timestamp (ms since 1970-01-01T00:00:00Z).
// 2440587.5 is the JD of the Unix epoch.
export function julianDate(nowMs: number): number {
  return nowMs / 86_400_000 + 2440587.5;
}

// Julian centuries since J2000.0 (JD 2451545.0 = 2000-01-01T12:00:00Z).
// Used by the sidereal time and obliquity formulas.
export function julianCenturiesJ2000(nowMs: number): number {
  return (julianDate(nowMs) - 2451545.0) / 36525;
}

// Days since 2000 Jan 0.0 UT (JD 2451543.5 = 1999-12-31T00:00:00Z), the epoch
// used by the orbital-elements tables in orbitalElements.ts (Paul Schlyter's
// widely-used "planetary positions" convention).
export function daysSinceElementsEpoch(nowMs: number): number {
  return julianDate(nowMs) - 2451543.5;
}

// Mean obliquity of the ecliptic (IAU low-precision linear term), degrees.
export function meanObliquityDeg(nowMs: number): number {
  const T = julianCenturiesJ2000(nowMs);
  return 23.4392911 - 0.0130042 * T;
}
