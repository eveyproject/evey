import { usePlaybackServiceGetSeriesPlayTarget, type V1MediaItem } from "@evey/api";
import { useAuthStore } from "@evey/store";

export interface SeriesPlayTarget {
  /** The episode Play starts; undefined while loading or on a server too old to answer. */
  episode?: V1MediaItem;
  /** The profile already played an episode of this series: say "Resume". */
  started: boolean;
  isLoading: boolean;
}

// The episode Play on a series starts, decided by the server for the active
// profile: the last one left unfinished, else the next one across seasons,
// else the first. staleTime 0: coming back from the player must not show the
// episode that was just finished.
export function useSeriesPlayTarget(seriesId: string | undefined): SeriesPlayTarget {
  const profileId = useAuthStore((s) => s.profileId) ?? "";
  const q = usePlaybackServiceGetSeriesPlayTarget(
    seriesId ?? "",
    profileId ? { profileId } : undefined,
    { query: { enabled: !!seriesId, staleTime: 0, retry: false } },
  );
  return { episode: q.data?.item?.id ? q.data.item : undefined, started: !!q.data?.started, isLoading: q.isLoading };
}
