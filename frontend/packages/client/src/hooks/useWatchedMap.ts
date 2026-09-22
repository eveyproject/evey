import { useCallback, useMemo } from "react";
import { progressPercent } from "../lib/progressPercent";
import type { V1WatchProgress } from "@evey/api";
import { usePlaybackServiceGetWatchHistory, WATCH_HISTORY_OVERLAY_LIMIT } from "@evey/api";
import type { V1GetWatchHistoryResponse } from "@evey/api";

// useIsWatched is the per-card subscription: it shares the same history query
// as useWatchedMap but selects a single boolean, so a refetch only re-renders
// the cards whose watched state actually changed. useWatchedMap hands every
// subscriber a NEW Map on each resolution - fine for one list-level filter,
// pathological for 100+ mounted cards.
export function useIsWatched(id: string | undefined) {
  const select = useCallback(
    (data: V1GetWatchHistoryResponse) =>
      !!id && (data?.items ?? []).some((it) => it.mediaItemId === id && it.played === true),
    [id],
  );
  const { data } = usePlaybackServiceGetWatchHistory(
    { limit: WATCH_HISTORY_OVERLAY_LIMIT },
    { query: { staleTime: 30_000, select } },
  );
  return data === true;
}

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

/**
 * The watched flag and the progress percent, keyed by media id, from one
 * watch-history response. What a grid needs to badge its cards.
 */
export function buildWatchProgressMaps(items: readonly V1WatchProgress[]): {
  watchedMap: Map<string, boolean>;
  percentMap: Map<string, number>;
} {
  const watchedMap = new Map<string, boolean>();
  const percentMap = new Map<string, number>();
  for (const w of items) {
    if (w.mediaItemId && w.played) watchedMap.set(w.mediaItemId, true);
    if (w.mediaItemId && w.percentComplete != null) percentMap.set(w.mediaItemId, progressPercent(w.percentComplete));
  }
  return { watchedMap, percentMap };
}

/** The maps above, fetched: one request the whole screen shares. */
export function useWatchProgressMaps() {
  const { data } = usePlaybackServiceGetWatchHistory(
    { limit: WATCH_HISTORY_OVERLAY_LIMIT },
    { query: { staleTime: 30_000 } },
  );
  return useMemo(() => buildWatchProgressMaps(data?.items ?? []), [data]);
}
