import type { LibraryServiceListMediaItemsType } from "./generated/library.schemas";

// Library business rules, defined ONCE for web, mobile and TV.
//
// These used to be copied per app with a "Mirrors web's X" comment as the only
// thing claiming they stayed in sync. They did not: on 2026-07-28 mobile and TV
// had copied DEFAULT_TYPE_FILTER but never DEDICATED_ROUTES, declared five lines
// above it in the same web file, so tapping a music or photo library opened an
// empty media grid on both. rulesGuard.test.ts fails the build if an app
// redeclares any of them locally again.

// Library types that are NOT browsed through the generic media grid, because
// their content is not media_items at all: music lives in the music domain
// (GetAlbums/GetArtists/ListTracks, each keyed by library_id) and photos in the
// photo domain. Sending them to /library/<id> queries ListMediaItems, which
// correctly returns zero rows for them - an empty screen over a full library.
export const DEDICATED_ROUTES: Readonly<Record<string, string>> = {
  LIBRARY_TYPE_PHOTOS: "/photos",
  LIBRARY_TYPE_MUSIC: "/music",
};

// The dedicated route for a library type, or undefined to browse it normally.
//
// `supportedRoutes` is what the CALLING app actually implements: the TV has a
// music screen but no photos screen, and redirecting to a route it does not
// have would trade an empty grid for a dead end. Passing the set explicitly
// keeps that per-app truth visible instead of hidden in a silent fallback.
export function dedicatedRouteFor(
  libraryType: string | undefined,
  supportedRoutes: readonly string[],
): string | undefined {
  const route = DEDICATED_ROUTES[libraryType ?? ""];
  return route && supportedRoutes.includes(route) ? route : undefined;
}

// Whether a library type's item count can be read from ListMediaItems. For the
// dedicated-route types it cannot: the honest display is no count at all,
// rather than a confident "0 items" over a library holding three albums.
export function hasMediaItemCount(libraryType: string | undefined): boolean {
  return !DEDICATED_ROUTES[libraryType ?? ""];
}

// A shows library opens on series only (not a series/season/episode mix), a
// movies library on movies only. Other types have no default.
export const DEFAULT_TYPE_FILTER: Partial<Record<string, LibraryServiceListMediaItemsType>> = {
  LIBRARY_TYPE_SHOWS: "MEDIA_ITEM_TYPE_SERIES",
  LIBRARY_TYPE_MOVIES: "MEDIA_ITEM_TYPE_MOVIE",
};

export function defaultTypeFilterFor(
  libraryType: string | undefined,
): LibraryServiceListMediaItemsType | "" {
  return DEFAULT_TYPE_FILTER[libraryType ?? ""] ?? "";
}

// In-library title search returns a single relevance-ranked page - there is no
// cursor to scroll through, so the limit IS the result set.
export const IN_LIBRARY_SEARCH_LIMIT = 120;

// How much watch history to pull for the watched/progress overlay on cards.
export const WATCH_HISTORY_OVERLAY_LIMIT = 500;

// --- Fetch-until-filled (client-only facets narrowing a short page) ---

// Client-only facets (resolution, codec, audio/subtitle language) have no
// server parameter, so they narrow an already-fetched page. That can leave a
// list too short to reach the scroll bottom and trigger the normal load-more,
// stranding the user on a near-empty grid while matching items sit further in
// the library. Keep fetching until enough matches are visible, bounded so a
// facet matching nothing cannot walk an entire 8k-item library.
export const AUTO_FETCH_FILL_TARGET = 24;
export const AUTO_FETCH_ITEM_CAP = 500;

export interface ShouldAutoFetchMoreInput {
  hasLocalFilters: boolean;
  hasMore: boolean;
  isFetching: boolean;
  filteredCount: number;
  loadedCount: number;
  fillTarget?: number;
  itemCap?: number;
}

export function shouldAutoFetchMore(input: ShouldAutoFetchMoreInput): boolean {
  const fillTarget = input.fillTarget ?? AUTO_FETCH_FILL_TARGET;
  const itemCap = input.itemCap ?? AUTO_FETCH_ITEM_CAP;
  if (!input.hasLocalFilters || !input.hasMore || input.isFetching) return false;
  if (input.filteredCount >= fillTarget) return false;
  if (input.loadedCount >= itemCap) return false;
  return true;
}
