// B-V color index -> a plausible RGB tint for rendering star points.
// B-V is the standard proxy for a star's surface temperature: negative
// (blue-white, hot O/B stars) through ~0 (white, A/F) to positive
// (orange/red, cooler K/M stars). This is a perceptual approximation (a
// small set of control colors, linearly interpolated), not a physical
// blackbody spectrum integration - plenty for tinting a point sprite.

export type RGB = [number, number, number];

const STOPS: Array<[number, RGB]> = [
  [-0.4, [0.61, 0.7, 1.0]],   // hot blue-white (O/B)
  [0.0, [0.83, 0.87, 1.0]],   // blue-white (A)
  [0.4, [1.0, 1.0, 0.98]],    // white (F)
  [0.7, [1.0, 0.94, 0.8]],    // yellow-white (G, Sun-like)
  [1.2, [1.0, 0.82, 0.6]],    // orange (K)
  [2.0, [1.0, 0.64, 0.45]],   // red-orange (M)
];

export function bvToRgb(bv: number): RGB {
  if (bv <= STOPS[0][0]) return STOPS[0][1];
  if (bv >= STOPS[STOPS.length - 1][0]) return STOPS[STOPS.length - 1][1];
  for (let i = 0; i < STOPS.length - 1; i++) {
    const [bv0, c0] = STOPS[i];
    const [bv1, c1] = STOPS[i + 1];
    if (bv >= bv0 && bv <= bv1) {
      const t = (bv - bv0) / (bv1 - bv0);
      return [
        c0[0] + (c1[0] - c0[0]) * t,
        c0[1] + (c1[1] - c0[1]) * t,
        c0[2] + (c1[2] - c0[2]) * t,
      ];
    }
  }
  return STOPS[STOPS.length - 1][1]; // unreachable, keeps TS happy
}
