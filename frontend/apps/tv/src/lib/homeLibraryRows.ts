import type { V1Library } from "@evey/api";

// The TV home screen adds one "recently added" row per library, after the
// three transverse rows (continue watching / up next / recently added
// overall). Each row is its own network request; firing one per library
// unconditionally would not scale to a large catalog on the NAS-class
// hardware Evey targets, so the row count is capped here rather than left
// unbounded.
export const MAX_HOME_LIBRARY_ROWS = 6;
export const HOME_LIBRARY_ROW_PAGE_SIZE = 20;

// A card in a home row navigates to /media/<id>, the VIDEO detail screen
// (resume, seasons, episodes). Only libraries whose items that screen actually
// fits get a row: a photo or book card would land the viewer on a page built
// for something else, and music already has its own tab with album/artist
// screens. Widen this list only alongside a detail screen that suits the type.
const ROWABLE_LIBRARY_TYPES = new Set(["LIBRARY_TYPE_MOVIES", "LIBRARY_TYPE_SHOWS"]);

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
