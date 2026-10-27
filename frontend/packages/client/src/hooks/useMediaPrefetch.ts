import { useCallback } from "react";
import { useQueryClient, type FetchQueryOptions } from "@tanstack/react-query";
import { getLibraryServiceGetMediaItemQueryOptions } from "@evey/api";

// How long the focus rests on a card before its page is fetched. The D-pad
// crosses a row a card at a time: shorter, and every card it passes over
// would cost a request. A pointer hovering a card means it more: the web
// passes a shorter wait.
export const PREFETCH_DWELL_MS = 250;

// One pending prefetch for the whole app: landing on the next card cancels
// the one the focus just left.
let pending: ReturnType<typeof setTimeout> | null = null;

/**
 * Fetches the page behind a card while the focus (or the pointer) rests on it,
 * so the page opens on data already there - the web did it on hover, the
 * televisions waited for OK and then for the request. Same query key as the
 * page, so the screen reads the cache.
 */
export function useMediaPrefetch(dwellMs = PREFETCH_DWELL_MS) {
  const qc = useQueryClient();

  const cancelPrefetch = useCallback(() => {
    if (pending) clearTimeout(pending);
    pending = null;
  }, []);

  /** Any page's queries: a library's first page, a season's episodes. */
  const prefetchQueriesOnFocus = useCallback(
    (queries: FetchQueryOptions[]) => {
      cancelPrefetch();
      if (queries.length === 0) return;
      pending = setTimeout(() => {
        pending = null;
        for (const q of queries) void qc.prefetchQuery(q);
      }, dwellMs);
    },
    [qc, dwellMs, cancelPrefetch],
  );

  /** A title's page. */
  const prefetchOnFocus = useCallback(
    (id: string | undefined) =>
      prefetchQueriesOnFocus(id ? [getLibraryServiceGetMediaItemQueryOptions(id) as FetchQueryOptions] : []),
    [prefetchQueriesOnFocus],
  );

  return { prefetchOnFocus, prefetchQueriesOnFocus, cancelPrefetch };
}
