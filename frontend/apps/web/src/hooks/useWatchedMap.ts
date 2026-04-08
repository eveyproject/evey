import { useMemo } from "react";
import { usePlaybackServiceGetWatchHistory } from "@evey/api";

export function useWatchedMap() {
  const { data } = usePlaybackServiceGetWatchHistory(
    { limit: 500 },
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
