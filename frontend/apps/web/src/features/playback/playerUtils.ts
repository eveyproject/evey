// nominalQualityTier lives in @evey/api now: mobile needs the same rounding to
// read the same manifest, and a second copy of a rule like this is how the two
// players end up labelling the same rung differently. Re-exported here so the
// call sites in this folder keep importing from where they always did.
export { nominalQualityTier } from "@evey/api";

// playerCloseTarget picks the media page the player lands on when it cannot
// simply pop its own history entry (deep link, hard refresh).
//
// An episode returns to its SEASON, because that is the page that lists it.
// Anything else (a film, a one-off) returns to its own page.
export function playerCloseTarget(
  mediaId: string,
  item?: { type?: string; parentId?: string },
): string {
  if (item?.type === "MEDIA_ITEM_TYPE_EPISODE" && item.parentId) return item.parentId;
  return mediaId;
}

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

export type PlayerExit = { action: "back" } | { action: "replace"; mediaId: string };

// playerExit decides HOW the player leaves, and the choice is about the history
// stack, not about the destination.
//
// Closing used to navigate FORWARD to the media page. That left the player's
// own entry in the back stack, so the media page's "Retour" walked straight
// back into the player and autoplayed the episode again - the bug this
// replaces. Popping the entry is what "closing" actually means.
//
// Going back is only correct because episode-to-episode navigation REPLACES
// (see handleEpisodeChange): the entry behind the player is therefore the page
// playback was launched from, never the previous episode - which is what made
// back() the wrong call when a binge pushed one entry per episode. The two
// belong together; changing one without the other reopens one of the two bugs.
//
// With nothing to pop, land on the media page and REPLACE, so a player entry is
// never left behind on that path either.
export function playerExit(
  canGoBack: boolean,
  mediaId: string,
  item?: { type?: string; parentId?: string },
): PlayerExit {
  if (canGoBack) return { action: "back" };
  return { action: "replace", mediaId: playerCloseTarget(mediaId, item) };
}
