// nominalQualityTier lives in @evey/api now: mobile needs the same rounding to
// read the same manifest, and a second copy of a rule like this is how the two
// players end up labelling the same rung differently. Re-exported here so the
// call sites in this folder keep importing from where they always did.
export { nominalQualityTier } from "@evey/api";

// Where closing the player lands: shared with the phone and the television
// (packages/client/src/lib/playerExit.ts), re-exported for this folder.
export { playerCloseTarget, playerExit, type PlayerExit } from "@evey/client";

// Where playback begins: shared with the phone and the televisions
// (packages/client/src/lib/resumePosition.ts).
export { resolveStartPositionMs } from "@evey/client";


/** One entry of the player's episode list; keeps the season number so the
 *  prev/next tooltips and the next-episode card can print "S01E04". */
export function toEpisodeNavItem(
  ep: { id?: string; title?: string; indexNumber?: number; seasonIndexNumber?: number },
  fallbackSeasonNumber?: number,
) {
  return {
    id: ep.id!,
    title: ep.title ?? `Episode ${ep.indexNumber}`,
    indexNumber: ep.indexNumber,
    seasonIndexNumber: ep.seasonIndexNumber || fallbackSeasonNumber,
  };
}
