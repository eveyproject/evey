const QUALITY_TIERS = [360, 480, 720, 1080, 1440, 2160];

// nominalQualityTier maps an ENCODED height to the nearest standard quality
// tier. A widescreen master (e.g. 2.40:1) is scaled to a fixed rung WIDTH with
// the height left to preserve aspect, so the "1080p" rung of a 1920×800 film is
// actually 1920×798 - and the raw height (798p) reads as a bug to users. Round
// it to the nearest familiar label (798 → 720p) for display only; playback and
// rung selection still key off the exact height.
export function nominalQualityTier(height: number): number {
  if (!height || height <= 0) return 0;
  return QUALITY_TIERS.reduce((best, tier) =>
    Math.abs(tier - height) < Math.abs(best - height) ? tier : best,
  );
}

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
