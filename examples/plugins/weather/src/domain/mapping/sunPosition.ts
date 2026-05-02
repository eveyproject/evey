// Simplified sun position approximation.
// Altitude = sin((hour - 6) / 12 * π), ignoring latitude/longitude.
// Good enough for visual coherence (sun low morning/evening, high noon, below horizon at night).

const TWO_PI = Math.PI * 2;

export interface SunPosition {
  altitude: number; // -π/2..π/2
  azimuth: number;  // -π..π (east=0, south=π/2 in northern hemisphere)
}

export function sunPosition(now: Date): SunPosition {
  const hours = now.getHours() + now.getMinutes() / 60;
  // altitude: peaks at noon (hour=12), minimum at midnight
  const altitude = Math.sin(((hours - 6) / 12) * Math.PI);
  // azimuth: east at sunrise, south at noon, west at sunset
  const azimuth = ((hours / 24) * TWO_PI) - Math.PI;
  return { altitude, azimuth };
}

export function moonPhase(now: Date): number {
  // Approx synodic period 29.53 days. Reference new moon: 2000-01-06.
  const refNewMoon = new Date("2000-01-06T18:14:00Z").getTime();
  const synodicMs = 29.530588853 * 24 * 3600 * 1000;
  return ((now.getTime() - refNewMoon) % synodicMs) / synodicMs;
}
