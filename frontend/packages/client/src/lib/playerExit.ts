// Where closing a player lands, the same rule on every client (web, phone,
// television used to keep a copy each).
//
// Closing the player POPS its own history entry. Navigating forward to the
// media page instead leaves the player in the back stack, so the media page's
// own back action walks right back into it and autoplays again. Popping is
// only correct because episode-to-episode navigation REPLACES the player
// entry: the entry behind the player is always the page playback was launched
// from, never an episode. The two belong together.
//
// With nothing to pop (deep link, notification, a device "loadAndPlay"), land
// on the media page and REPLACE, so a player entry is never left behind.

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

export function playerExit(
  canGoBack: boolean,
  mediaId: string,
  item?: { type?: string; parentId?: string },
): PlayerExit {
  if (canGoBack) return { action: "back" };
  return { action: "replace", mediaId: playerCloseTarget(mediaId, item) };
}
