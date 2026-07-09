// The poster wall's geometry: a 2-3 row perspective grid, laid out once per
// mount (or whenever the media selection changes) - never recomputed inside
// the render loop. Coordinates are a normalized "wall space": x spans one
// seamless horizontal tile centered on 0, y grows with row (row 0 = front,
// closest, biggest), scale/z drive the perspective falloff and parallax.

export interface WallSlot {
  index: number;
  row: number;
  col: number;
  x: number;
  y: number;
  scale: number;
  z: number; // 0 = front row, 1 = back row
}

export interface WallLayout {
  slots: WallSlot[];
  rows: number;
  colsPerRow: number;
  // Width of one seamless horizontal tile: scrolling by exactly this amount
  // loops back to the same visual arrangement, so a renderer can wrap the
  // scroll offset with `offset % period` and repeat each slot at
  // x, x - period, x + period to cover the viewport continuously.
  period: number;
}
