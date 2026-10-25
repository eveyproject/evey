import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { getLibraryServiceGetMediaItemQueryOptions } from "@evey/api";

// How long the focus rests on a card before its page is fetched. The D-pad
// crosses a row a card at a time: shorter, and every card it passes over
// would cost a request.
export const PREFETCH_DWELL_MS = 250;

// One pending prefetch for the whole app: landing on the next card cancels
// the one the focus just left.
let pending: ReturnType<typeof setTimeout> | null = null;

/**
 * The web fetches a title's page while the pointer rests on its card, so the
 * page opens on data already there; the television waited for OK, then for
 * the request, and the page took seconds to come up (2026-10-25). Same here
 * on focus: the page's own query key, so the screen reads the cache.
 */
export function useMediaPrefetch() {
  const qc = useQueryClient();

  const prefetchOnFocus = useCallback(
    (id: string | undefined) => {
      if (pending) clearTimeout(pending);
      pending = null;
      if (!id) return;
      pending = setTimeout(() => {
        pending = null;
        void qc.prefetchQuery(getLibraryServiceGetMediaItemQueryOptions(id));
      }, PREFETCH_DWELL_MS);
    },
    [qc],
  );

  const cancelPrefetch = useCallback(() => {
    if (pending) clearTimeout(pending);
    pending = null;
  }, []);

  return { prefetchOnFocus, cancelPrefetch };
}
