// When the proactive token refresh fires. A failed refresh (server restarting,
// laptop waking offline) keeps the same token, whose refresh time is already
// past: rescheduling at max(past, 0) spun every open tab on /auth/refresh with
// a zero delay - 879 calls a second, measured 2026-10-01. Failures back off.
export const REFRESH_MARGIN_MS = 2 * 60 * 1000;
const MAX_BACKOFF_MS = 60_000;

export function nextRefreshDelay(expiresMs: number, now: number, jitterMs: number, failures: number): number {
  const due = Math.max(expiresMs - REFRESH_MARGIN_MS + jitterMs - now, 0);
  if (failures <= 0) return due;
  return Math.max(due, Math.min(MAX_BACKOFF_MS, 1000 * 2 ** (failures - 1)));
}
