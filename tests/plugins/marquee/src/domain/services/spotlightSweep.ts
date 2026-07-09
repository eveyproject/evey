// The ambient "projector beam" that slowly sweeps the wall when no roulette
// is running: a triangle wave so it swings back and forth rather than
// snapping. Pure function of wall-clock time, so it needs no per-frame state
// of its own.

export function spotlightPhase(nowMs: number, periodMs: number): number {
  if (!(periodMs > 0)) return 0;
  const t = ((nowMs % periodMs) + periodMs) % periodMs / periodMs; // 0..1
  return t < 0.5 ? t * 2 : 2 - t * 2;
}

export function spotlightX(phase: number, halfWidth = 1): number {
  return (phase * 2 - 1) * halfWidth;
}
