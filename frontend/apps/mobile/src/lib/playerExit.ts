// Twin of the web player's exit helpers (apps/web .../playback/playerUtils.ts:
// playerCloseTarget / playerExit) - keep the SEMANTICS in sync when touching
// either side.
//
// Closing the player POPS its own history entry. Navigating forward to the
// media page instead leaves the player in the back stack, so the media page's
// own back button walks right back into it and autoplays again. Popping is
// only correct because episode-to-episode navigation REPLACES the player
// entry (handleNavigateEpisode uses router.replace) - the entry behind the
// player is always the page playback was launched from, never an episode.
//
// With nothing to pop (deep link, notification), land on the media page and
// REPLACE, so a player entry is never left behind on that path either.

// An episode returns to its SEASON, because that is the page that lists it.
// Anything else (a film, a one-off) returns to its own page.
export function playerCloseTarget(
  mediaId: string,
  item?: { type?: string; parentId?: string },
): string {
  if (item?.type === "MEDIA_ITEM_TYPE_EPISODE" && item.parentId) return item.parentId;
  return mediaId;
}

// Hardware back closes what is OPEN before it closes the PLAYER. The settings
// panel is a plain positioned View - unlike the sheets, which are native
// Modals whose onRequestClose intercepts the back press before this handler
// ever runs - so without this decision a back press with the panel up tears
// down playback instead of dismissing the panel.
export type BackPressAction = "close-settings" | "exit";

export function backPressAction(settingsOpen: boolean): BackPressAction {
  return settingsOpen ? "close-settings" : "exit";
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
