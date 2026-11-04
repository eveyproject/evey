import type { QueryClient, QueryKey } from "@tanstack/react-query";

// The home tab is never unmounted and its rows stay fresh for minutes (the
// client's default staleTime): a film added to a library while the television
// sat on the home never showed up there, notification or not, until the app
// was restarted.

// The lists the home rows read: every library row and the recommendations.
// A single title (`/api/v1/media/<id>`) is not a list and is left alone.
const HOME_LIST_ROOTS = new Set(["/api/v1/media", "/api/v1/feeds/recommended"]);

// Back on the home, rows younger than this are not read again: walking from a
// title page to the home and back must not refetch every row on a small NAS.
export const HOME_LISTS_MAX_AGE_MS = 60_000;

export function isHomeListQuery(key: QueryKey): boolean {
  return typeof key[0] === "string" && HOME_LIST_ROOTS.has(key[0]);
}

// refreshHomeLists re-reads the home's lists older than `olderThanMs` (all of
// them at 0). Rows keep their items on screen while the new page loads.
export function refreshHomeLists(qc: QueryClient, olderThanMs = 0, now = Date.now()): Promise<void> {
  return qc.invalidateQueries({
    predicate: (q) => isHomeListQuery(q.queryKey) && now - q.state.dataUpdatedAt >= olderThanMs,
  });
}
