import { useCallback } from "react";
import {
  useLibraryServiceListMediaItems,
  type V1MediaItem,
} from "@evey/api";

import { useUpNextCountdown } from "./useUpNextCountdown";


interface UseNextEpisodeArgs {
  media: V1MediaItem | undefined;
  currentTime: number;
  duration: number;
  onNavigate: (episodeId: string) => void;
  // Per-profile preference (player.settings.autoplayNext). When false, the
  // "up next" card still appears and stays put - it just never counts down
  // or navigates on its own; only playNow/playPrevious move to another
  // episode. Defaults to true, the behaviour every player had before the
  // preference existed.
  autoAdvance?: boolean;
  // Nobody answered the "still watching?" card in time: leave the player.
  onStillWatchingTimeout?: () => void;
}

export function useNextEpisode({
  media,
  currentTime,
  duration,
  onNavigate,
  autoAdvance = true,
  onStillWatchingTimeout,
}: UseNextEpisodeArgs) {
  const isEpisode = media?.type === "MEDIA_ITEM_TYPE_EPISODE";
  const parentId = media?.parentId;

  const { data: siblingResponse } = useLibraryServiceListMediaItems(
    isEpisode && parentId
      ? {
          parentId,
          type: "MEDIA_ITEM_TYPE_EPISODE",
          "pagination.pageSize": 500,
        }
      : undefined,
    { query: { enabled: isEpisode && !!parentId } },
  );

  const siblings = siblingResponse?.items
    ?.slice()
    .sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));

  const nextEpisode = (() => {
    if (!siblings || !media?.id) return null;
    const idx = siblings.findIndex((e) => e.id === media.id);
    if (idx < 0 || idx >= siblings.length - 1) return null;
    return siblings[idx + 1];
  })();

  // Manual "previous episode" control (see task item 2): the auto-advance only ever
  // goes forward, so this is computed independently and never drives the countdown.
  const previousEpisode = (() => {
    if (!siblings || !media?.id) return null;
    const idx = siblings.findIndex((e) => e.id === media.id);
    if (idx <= 0) return null;
    return siblings[idx - 1];
  })();

  const { visible, ended, countdown, stillWatching, playNow, cancel } = useUpNextCountdown({
    nextEpisodeId: nextEpisode?.id,
    currentTime,
    duration,
    onPlay: onNavigate,
    autoAdvance,
    onStillWatchingTimeout,
  });

  const playPrevious = useCallback(() => {
    if (previousEpisode?.id) onNavigate(previousEpisode.id);
  }, [previousEpisode, onNavigate]);

  return {
    visible,
    ended,
    countdown,
    stillWatching,
    nextEpisode,
    previousEpisode,
    playNow,
    playPrevious,
    cancel,
  };
}
