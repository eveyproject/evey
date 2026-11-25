import { useCallback, useMemo } from "react";
import { progressPercent } from "../lib/progressPercent";
import { showsWatched } from "../lib/resumePosition";
import type { V1WatchProgress } from "@evey/api";
import { usePlaybackServiceGetWatchHistory, WATCH_HISTORY_OVERLAY_LIMIT } from "@evey/api";
import type { V1GetWatchHistoryResponse, V1MediaUserData } from "@evey/api";

// useIsWatched is the per-card subscription: it shares the same history query
// as useWatchedMap but selects a single boolean, so a refetch only re-renders
// the cards whose watched state actually changed. useWatchedMap hands every
// subscriber a NEW Map on each resolution - fine for one list-level filter,
// pathological for 100+ mounted cards. It answers for the card's mark
// (showsWatched), not for the stored flag.
export function useIsWatched(id: string | undefined, enabled = true) {
  const select = useCallback(
    (data: V1GetWatchHistoryResponse) =>
      !!id && (data?.items ?? []).some((it) => it.mediaItemId === id && showsWatched(it)),
    [id],
  );
  const { data } = usePlaybackServiceGetWatchHistory(
    { limit: WATCH_HISTORY_OVERLAY_LIMIT },
    { query: { staleTime: 30_000, select, enabled } },
  );
  return data === true;
}

// The watched mark of a title is the server's: every title it returns carries
// user_data (MediaUserData), read by one rule (a series once every episode is
// done). The watch-history overlay below is only the answer of a server too
// old to send it - a title without user_data - and it lost titles past its
// 500-row window.
type WithUserData = { id?: string; userData?: V1MediaUserData | null };

/** Whether a list should count the title as seen (the stored flag). */
export function itemPlayed(item: WithUserData, legacy?: ReadonlyMap<string, boolean>): boolean {
  if (item.userData) return item.userData.played === true;
  return !!item.id && legacy?.get(item.id) === true;
}

/** Whether the title's card wears the watched mark (see showsWatched). */
export function itemShowsWatched(item: WithUserData, legacy?: ReadonlyMap<string, boolean>): boolean {
  if (item.userData) return showsWatched(item.userData);
  return !!item.id && legacy?.get(item.id) === true;
}

/** The title's progress in percent, 0 when none. */
export function itemPercent(item: WithUserData, legacy?: ReadonlyMap<string, number>): number {
  if (item.userData) return progressPercent(item.userData.percentComplete ?? 0);
  return (item.id && legacy?.get(item.id)) || 0;
}

/** What a list hides as already seen: the server's flag, or the overlay's. */
export function filterUnwatched<T extends WithUserData>(items: readonly T[], legacy?: ReadonlyMap<string, boolean>): T[] {
  return items.filter((it) => !itemPlayed(it, legacy));
}

/** A card's mark, reading the overlay only for a title without user_data. */
export function useItemShowsWatched(item: WithUserData): boolean {
  const legacy = useIsWatched(item.id, !item.userData);
  return item.userData ? showsWatched(item.userData) : legacy;
}

// useWatchedMap is the stored flag: what a list filters "already seen" by.
// A card's mark reads buildWatchProgressMaps instead.
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
 * The watched mark (showsWatched) and the progress percent, keyed by media id,
 * from one watch-history response. What a grid needs to badge its cards.
 */
export function buildWatchProgressMaps(items: readonly V1WatchProgress[]): {
  watchedMap: Map<string, boolean>;
  percentMap: Map<string, number>;
} {
  const watchedMap = new Map<string, boolean>();
  const percentMap = new Map<string, number>();
  for (const w of items) {
    if (w.mediaItemId && showsWatched(w)) watchedMap.set(w.mediaItemId, true);
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
