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
  progress?: ResumeProgress | null,
): number {
  if (restart) return 0;
  return resumableMs(progress);
}

type ResumeProgress = { positionMs?: string | number; durationMs?: string | number; played?: boolean };

// The server's own completion point (command_service.go: 9/10 of the runtime).
const COMPLETED_NUM = 9;
const COMPLETED_DEN = 10;

// resumableMs is the position a Resume would start at, 0 when there is
// nothing to resume. The media pages label their button with it, the players
// start there: both read the same rule.
//
// The played flag alone does not mean "at the end". Completion is sticky on
// the server: a title finished once keeps its flag while the position follows
// the next viewing. Reading the flag as "the end" sent every viewer who left a
// second viewing back to the start of the film. A played title is at its end
// only past the completion point, or when its length is unknown.
export function resumableMs(progress?: ResumeProgress | null): number {
  const positionMs = progress?.positionMs ? Number(progress.positionMs) : 0;
  if (!positionMs || positionMs <= 0) return 0;
  const durationMs = progress?.durationMs ? Number(progress.durationMs) : 0;
  if (durationMs > 0 && positionMs >= durationMs - RESUME_END_EPSILON_MS) return 0;
  if (progress?.played === true && (durationMs <= 0 || positionMs * COMPLETED_DEN >= durationMs * COMPLETED_NUM)) return 0;
  return positionMs;
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
  if (!first) {
    // A later stream (quality, audio or subtitle switch) continues where the
    // viewer is: the "played" flag of a title being watched again sent every
    // switch back to 0 (measured on the TV, 2026-09-25). Only the end stays a trap.
    return resolveStartPositionMs(false, wp && { ...wp, played: false });
  }
  return resolveStartPositionMs(!!initial?.restart, wp);
}
