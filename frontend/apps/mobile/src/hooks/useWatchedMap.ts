import { useMemo } from "react";
import { usePlaybackServiceGetWatchHistory, WATCH_HISTORY_OVERLAY_LIMIT } from "@evey/api";

/**
 * Mirror of web's useWatchedMap - fetches the last N watch-history entries
 * once and exposes a map<mediaItemId, boolean>. Used by media cards across
 * the home and library to render a "watched" badge.
 *
 * Limit = 500 mirrors web; server response is small (id+played+positionMs).
 * 30s staleTime keeps the request cheap when multiple sliders mount the
 * same map at the same time.
 */
export function useWatchedMap() {
  const { data } = usePlaybackServiceGetWatchHistory(
    { limit: WATCH_HISTORY_OVERLAY_LIMIT },
    { query: { staleTime: 30_000 } },
  );

  return useMemo(() => {
    const map = new Map<string, boolean>();
    for (const item of data?.items ?? []) {
      if (item.mediaItemId && item.played) {
        map.set(item.mediaItemId, true);
      }
    }
    return map;
  }, [data]);
}
