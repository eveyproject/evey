// Pure, side-effect-free helpers backing the mobile search screen's recent-
// searches list ((tabs)/search.tsx). Lives here (rather than a new src/lib
// file) so it stays inside this agent's whitelisted src/components/library/**
// directory - the logic itself has nothing to do with libraries, only with
// keeping the mobile client's storage-free history math testable and in
// parity with web's useRecentSearches.ts.

// Mirrors web's MAX (useRecentSearches.ts:4) - mobile previously kept 10.
export const MAX_SEARCH_HISTORY = 8;

// Adds a query to the front of the history, de-duplicating any existing
// occurrence and clamping to max entries. Mirrors web's push().
export function pushSearchHistory(history: string[], query: string, max: number = MAX_SEARCH_HISTORY): string[] {
  const trimmed = query.trim();
  if (!trimmed) return history;
  const filtered = history.filter((h) => h !== trimmed);
  return [trimmed, ...filtered].slice(0, max);
}

// Removes a single entry, leaving the rest of the history untouched. Mirrors
// web's remove() (useRecentSearches.ts:33-39) - mobile previously only had
// "clear all".
export function removeSearchHistoryEntry(history: string[], query: string): string[] {
  return history.filter((h) => h !== query);
}
