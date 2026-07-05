// Moon phase from the synodic period. 0 = new, 0.5 = full, 1 = new.

const REF_NEW_MOON_MS = Date.UTC(2000, 0, 6, 18, 14, 0);
const SYNODIC_MS = 29.530588853 * 24 * 3600 * 1000;

export function moonPhase(nowMs: number): number {
  const p = ((nowMs - REF_NEW_MOON_MS) % SYNODIC_MS) / SYNODIC_MS;
  return p < 0 ? p + 1 : p;
}
