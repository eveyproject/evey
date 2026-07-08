// Stereographic sky-dome projection: altitude/azimuth -> a flat unit disc.
// Zenith (alt=90) maps to the center; the horizon (alt=0) maps exactly to the
// unit circle (r=1); below the horizon r>1 (the renderer clips it there).
//
// View convention: looking straight up at the dome, North at the top,
// East to the right (a conventional star-chart layout, not a mirror-image
// "looking down" view).

import { toRad } from "./mathUtils";

export interface ProjectedPoint {
  x: number;
  y: number;
  visible: boolean; // above the horizon (with a small margin for refraction)
}

export function stereographicProject(altDeg: number, azDeg: number): ProjectedPoint {
  const zenithAngle = toRad(90 - altDeg);
  const r = Math.tan(zenithAngle / 2);
  const az = toRad(azDeg);
  return {
    x: r * Math.sin(az),
    y: r * Math.cos(az),
    visible: altDeg > -2,
  };
}
