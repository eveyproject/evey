import type { V1Library } from "@evey/api";

// The TV home screen adds one "recently added" row per library that the
// server's feed config does not already place. Each row is its own network
// request; firing one per library unconditionally would not scale to a large
// catalog on the NAS-class hardware Evey targets, so the count of these
// UNCONFIGURED rows is capped here. Libraries the operator placed in the feed
// config on purpose are not capped - see buildHomeSections.
export const MAX_HOME_LIBRARY_ROWS = 6;
export const HOME_LIBRARY_ROW_PAGE_SIZE = 20;

// A row only exists for a library type the TV has somewhere to SEND the
// viewer. Movies and shows go to /media/<id>; music albums to the album
// screen; photo albums to the photos screen. An unknown or future type gets
// no row - a card that lands nowhere is worse than an absent row. Widen this
// list only alongside a screen that suits the type.
const ROWABLE_LIBRARY_TYPES = new Set([
  "LIBRARY_TYPE_MOVIES",
  "LIBRARY_TYPE_SHOWS",
  "LIBRARY_TYPE_MUSIC",
  "LIBRARY_TYPE_PHOTOS",
]);

export interface HomeLibraryRow {
  id: string;
  name: string;
}

// Picks which libraries get a home-screen row: drops any library missing the
// id/name a row needs (it can neither be queried nor labeled) or whose type
// the row's destination screen doesn't fit, then truncates to
// MAX_HOME_LIBRARY_ROWS while preserving the server's order for the ones kept.
export function selectHomeLibraryRows(libraries: V1Library[]): HomeLibraryRow[] {
  const rows: HomeLibraryRow[] = [];
  for (const lib of libraries) {
    if (rows.length >= MAX_HOME_LIBRARY_ROWS) break;
    if (!lib.id || !lib.name) continue;
    if (!lib.type || !ROWABLE_LIBRARY_TYPES.has(lib.type)) continue;
    rows.push({ id: lib.id, name: lib.name });
  }
  return rows;
}
