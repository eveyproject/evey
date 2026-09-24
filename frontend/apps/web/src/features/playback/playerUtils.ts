// nominalQualityTier lives in @evey/api now: mobile needs the same rounding to
// read the same manifest, and a second copy of a rule like this is how the two
// players end up labelling the same rung differently. Re-exported here so the
// call sites in this folder keep importing from where they always did.
export { nominalQualityTier } from "@evey/api";

// Where closing the player lands: shared with the phone and the television
// (packages/client/src/lib/playerExit.ts), re-exported for this folder.
export { playerCloseTarget, playerExit, type PlayerExit } from "@evey/client";

// How close to the runtime still counts as "finished". A resume position within
// this window of the end (or a title the server already marked played) is
// treated as watched-to-the-end.
const RESUME_END_EPSILON_MS = 15_000;

// resolveStartPositionMs decides where playback begins.
//
// A saved resume position sitting AT the end of a finished title is a trap:
// seeking <video> there plays nothing (EOF), hls.js buffer-stalls on a black
// screen, and the media page shows no "start over" button for a played item -
// so its sole Play button drops you at the end with no way forward (reported on
// a film watched to 100% overnight: the player never started). A watched-to-the-
// end title must restart from 0, the way every media app does; genuine mid-title
// progress still resumes.
export function resolveStartPositionMs(
  restart: boolean,
  progress?: { positionMs?: string; durationMs?: string; played?: boolean } | null,
): number {
  if (restart) return 0;
  const positionMs = progress?.positionMs ? Number(progress.positionMs) : 0;
  if (!positionMs || positionMs <= 0) return 0;
  const durationMs = progress?.durationMs ? Number(progress.durationMs) : 0;
  const finished =
    progress?.played === true || (durationMs > 0 && positionMs >= durationMs - RESUME_END_EPSILON_MS);
  return finished ? 0 : positionMs;
}

