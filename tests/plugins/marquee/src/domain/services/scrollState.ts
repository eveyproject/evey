// Ambient lateral drift of the wall: a single wrapped scalar the renderer
// advances every frame. Kept pure so the wrap-around math (the part that
// actually matters - overshoot, negative speed, degenerate period) is
// testable without a GL context.

export function advanceScroll(offset: number, dtSeconds: number, speedPerSec: number, period: number): number {
  if (!(period > 0)) return 0;
  const next = (offset + dtSeconds * speedPerSec) % period;
  return next < 0 ? next + period : next;
}
