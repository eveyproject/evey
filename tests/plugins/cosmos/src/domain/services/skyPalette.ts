// Sky brightness/star-visibility model driven purely by the Sun's real
// altitude - no lookup tables, no "is it day" boolean from an API (unlike
// weather-v2, which needs the Open-Meteo daily response for this). The
// thresholds match the standard astronomical twilight definitions:
//   civil twilight ends at sunAlt = -6deg, nautical at -12deg,
//   astronomical (stars fully out) at -18deg.

import { clamp } from "./mathUtils";

function smoothstep(edge0: number, edge1: number, x: number): number {
  const t = clamp((x - edge0) / (edge1 - edge0), 0, 1);
  return t * t * (3 - 2 * t);
}

// 0 in daylight, ramping up through twilight, 1 once astronomical night has
// fully set in (sunAlt <= -18deg).
export function starVisibility(sunAltDeg: number): number {
  return smoothstep(2, -18, sunAltDeg);
}

// 0..1, peaks around sunset/sunrise (roughly -4..+4deg), fades to 0 in full
// daylight or deep night - drives the warm horizon glow.
export function twilightWarmth(sunAltDeg: number): number {
  const rising = smoothstep(-10, -2, sunAltDeg);
  const falling = 1 - smoothstep(4, 12, sunAltDeg);
  return clamp(Math.min(rising, falling), 0, 1);
}

export type SkyMood = "day" | "twilight" | "night";

export function skyMood(sunAltDeg: number): SkyMood {
  if (sunAltDeg > 2) return "day";
  if (sunAltDeg > -12) return "twilight";
  return "night";
}
