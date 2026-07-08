// Small shared numeric helpers. Pure, allocation-free, no DOM.

export const DEG2RAD = Math.PI / 180;
export const RAD2DEG = 180 / Math.PI;

export function toRad(deg: number): number {
  return deg * DEG2RAD;
}

export function toDeg(rad: number): number {
  return rad * RAD2DEG;
}

// Wraps to [0, 360).
export function normalizeDeg(deg: number): number {
  const r = deg % 360;
  return r < 0 ? r + 360 : r;
}

export function clamp(v: number, lo: number, hi: number): number {
  return v < lo ? lo : v > hi ? hi : v;
}
