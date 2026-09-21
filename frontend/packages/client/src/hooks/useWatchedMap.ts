import { useCallback, useMemo } from "react";
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
