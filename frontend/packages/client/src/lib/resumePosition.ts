// How close to the runtime still counts as "finished". A resume position within
// this window of the end (or a title the server already marked played) is
// treated as watched-to-the-end.
export const RESUME_END_EPSILON_MS = 15_000;

// resolveStartPositionMs decides where playback begins, on every client.
//
// A saved resume position sitting AT the end of a finished title is a trap:
// seeking there plays nothing (EOF) and the title's sole Play button drops the
// viewer at the end with no way forward. A watched-to-the-end title restarts
// from 0; genuine mid-title progress still resumes. Only the web applied this
// until 2026-09-24: the browser television opened a played title at its end.
export function resolveStartPositionMs(
  restart: boolean,
  progress?: { positionMs?: string | number; durationMs?: string | number; played?: boolean } | null,
): number {
  if (restart) return 0;
  const positionMs = progress?.positionMs ? Number(progress.positionMs) : 0;
  if (!positionMs || positionMs <= 0) return 0;
  const durationMs = progress?.durationMs ? Number(progress.durationMs) : 0;
  const finished =
    progress?.played === true || (durationMs > 0 && positionMs >= durationMs - RESUME_END_EPSILON_MS);
  return finished ? 0 : positionMs;
}

// firstStartMs: the position the first stream of a session starts at (the
// restart and cast hints apply once), then the saved progress by the rule of
// every player (resolveStartPositionMs).
export function firstStartMs(
  firstRef: { current: boolean },
  initial: { restart?: boolean; startSec?: number } | undefined,
  wp: { positionMs?: string; durationMs?: string; played?: boolean } | null | undefined,
): number {
  const first = firstRef.current;
  firstRef.current = false;
  if (first && initial?.startSec && initial.startSec > 0) return Math.round(initial.startSec * 1000);
  return resolveStartPositionMs(first && !!initial?.restart, wp);
}
