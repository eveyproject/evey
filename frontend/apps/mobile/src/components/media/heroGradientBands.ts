// React Native has no CSS gradient primitive, and pulling in a native module
// (expo-linear-gradient) for this one screen isn't worth it here. This
// approximates a smooth linear fade with N discrete flat-opacity layers of
// the SAME height, stacked bottom-to-top: opacity climbs from 0 (fully
// see-through, at the top of the hero) to 1 (fully opaque, at the bottom) so
// the backdrop blends into the page background with no visible seam.
//
// The previous 4-band version only covered the bottom 60% of the hero and
// jumped in big linear steps - both of which read as "a visible band, a
// brutal cut" (the reported bug): nothing above the 60% mark, and coarse
// jumps below it. Spanning the FULL height fixes the cutoff; an eased
// (quadratic) curve rather than a linear one plus a much higher step count
// fixes the banding - perceived brightness isn't linear in alpha over a
// photo, so a straight ramp still reads as two visible bands near the ends.
export function heroGradientBands(steps: number): number[] {
  if (!Number.isFinite(steps) || steps <= 0) return [];
  const bands: number[] = [];
  for (let i = 1; i <= steps; i++) {
    const t = i / steps;
    bands.push(t * t);
  }
  return bands;
}
