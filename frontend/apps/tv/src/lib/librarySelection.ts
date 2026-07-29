import type { V1MediaItem, LibraryServiceListMediaItemsType } from "@evey/api";

// "Play all" and "shuffle" both act on whatever the library screen is
// CURRENTLY showing (sort, genre, type filter, search) - never on the whole
// library behind the user's back. Everything here is pure and takes that
// selection as plain input, so the sit-down-and-press-play behaviour can be
// asserted without rendering the screen or touching the network.

// Only a movie or an episode carries a video stream of its own. A series or
// season row is a navigational grouping with nothing to hand the player, so
// it is dropped rather than pushed to a route that has nothing to play.
const PLAYABLE_TYPES = new Set(["MEDIA_ITEM_TYPE_MOVIE", "MEDIA_ITEM_TYPE_EPISODE"]);

export function isPlayableMediaType(type?: string): boolean {
  return !!type && PLAYABLE_TYPES.has(type);
}

// Whether "play all" makes sense for the library/type-filter combination
// active on screen, BEFORE any items have even loaded. A shows library lists
// series (or, once filtered, seasons) by default - neither is playable - so
// the control only appears once the type filter narrows the view to episodes.
// A movies library's items are always directly playable.
export function canPlayAllForSelection(
  _isShowLibrary: boolean,
  typeFilter?: LibraryServiceListMediaItemsType | "",
): boolean {
  // Only an EPISODE selection. The player has no external queue: its sole
  // auto-advance is useNextEpisode, which chains episodes within a season
  // (isEpisode && parentId) and nothing else. On a movie selection the button
  // would therefore play exactly one film and stop - a control whose label
  // lies, and on a TV the viewer has no way to discover that. Pressing the
  // first card already does that much, so there is nothing to offer there.
  // Widen this the day the player gains a real queue, not before.
  return typeFilter === "MEDIA_ITEM_TYPE_EPISODE";
}

// The ordered "play all" queue for the current selection. `items` must
// already be whatever sort/filter/search produced on screen - this function
// never re-sorts or re-fetches, it only drops rows with nothing to play.
export function buildPlayAllQueue(items: V1MediaItem[]): string[] {
  return items.filter((item) => item.id && isPlayableMediaType(item.type)).map((item) => item.id as string);
}

// A random pick within an already-loaded, already-filtered selection - used
// where the loaded page IS the full result set (e.g. search, which returns a
// single fixed-size page with no cursor to page through). It intentionally
// does NOT reach for more than what it is given: a caller shuffling across a
// paginated, only-partially-loaded list must fetch a wider set (or ask the
// server for a random page, as this screen's library-wide shuffle already
// does) rather than rely on this picking outside what it was handed.
export function pickRandomItem<T>(items: T[]): T | undefined {
  if (items.length === 0) return undefined;
  return items[Math.floor(Math.random() * items.length)];
}
