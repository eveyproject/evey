// Shared wall-geometry constants: the application layer feeds these into
// buildWallLayout(), the render adapter derives poster hit-box/quad sizes
// from the same values. Keeping a single source of truth here (rather than
// duplicating the numbers in both layers) is what keeps the layout the
// application builds and the boxes the renderer draws/hit-tests in sync.

export const WALL_GEOMETRY = {
  colSpacing: 1.0,
  rowRise: 0.42,
  rowScaleFalloff: 0.82,
  rowStaggerFraction: 0.5,
  // Typical movie/show poster width:height ratio.
  posterAspect: 2 / 3,
} as const;
