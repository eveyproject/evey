import type { V1Library } from "@evey/api";

// The TV home screen adds one "recently added" row per library, after the
// three transverse rows (continue watching / up next / recently added
// overall). Each row is its own network request; firing one per library
// unconditionally would not scale to a large catalog on the NAS-class
// hardware Evey targets, so the row count is capped here rather than left
// unbounded.
export const MAX_HOME_LIBRARY_ROWS = 6;
export const HOME_LIBRARY_ROW_PAGE_SIZE = 20;

export interface HomeLibraryRow {
  id: string;
  name: string;
}

// Picks which libraries get a home-screen row: drops any library missing the
// id/name a row needs (it can neither be queried nor labeled), then
// truncates to MAX_HOME_LIBRARY_ROWS while preserving the server's order for
// the ones that are kept.
export function selectHomeLibraryRows(libraries: V1Library[]): HomeLibraryRow[] {
  const rows: HomeLibraryRow[] = [];
  for (const lib of libraries) {
    if (rows.length >= MAX_HOME_LIBRARY_ROWS) break;
    if (!lib.id || !lib.name) continue;
    rows.push({ id: lib.id, name: lib.name });
  }
  return rows;
}
