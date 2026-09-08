import { useEffect, useRef, useState, useCallback } from "react";
import {
  useLibraryServiceListMediaItems,
  type V1MediaItem,
} from "@evey/api";

const CREDITS_THRESHOLD_SEC = 30;
export const COUNTDOWN_SEC = 5;

interface UseNextEpisodeArgs {
  media: V1MediaItem | undefined;
  currentTime: number;
  duration: number;
  onNavigate: (episodeId: string) => void;
  // When false, the next-episode card still shows up ("Play now" stays
  // available) but no countdown starts and onNavigate is never called on
  // its own - only playNow() triggers it. Per-profile preference, default
  // on to match the behaviour before the setting existed.
  autoAdvance?: boolean;
}

export function useNextEpisode({
  media,
  currentTime,
  duration,
  onNavigate,
  autoAdvance = true,
}: UseNextEpisodeArgs) {
  const [countdown, setCountdown] = useState(COUNTDOWN_SEC);
  const [cancelled, setCancelled] = useState(false);
  const [ended, setEnded] = useState(false);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

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

  const isInCredits =
    duration > 0 && currentTime > 0 && duration - currentTime <= CREDITS_THRESHOLD_SEC;
  const videoEnded = duration > 0 && currentTime >= duration - 0.5;

  useEffect(() => {
    if (videoEnded && !ended) setEnded(true);
  }, [videoEnded, ended]);

  const visible = !!nextEpisode && !cancelled && (isInCredits || ended);

  useEffect(() => {
    if (!ended || cancelled || !nextEpisode || !autoAdvance) return;
    setCountdown(COUNTDOWN_SEC);
    intervalRef.current = setInterval(() => {
      setCountdown((prev) => prev - 1);
    }, 1000);
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, [ended, cancelled, nextEpisode, autoAdvance]);

  useEffect(() => {
    if (!autoAdvance) return;
    if (countdown <= 0 && nextEpisode && !cancelled) {
      if (intervalRef.current) clearInterval(intervalRef.current);
      if (!nextEpisode?.id) return;
      onNavigate(nextEpisode.id);
    }
  }, [countdown, nextEpisode, cancelled, onNavigate, autoAdvance]);

  // Reset on episode change
  useEffect(() => {
    setCancelled(false);
    setEnded(false);
  }, [media?.id]);

  const playNow = useCallback(() => {
    if (intervalRef.current) clearInterval(intervalRef.current);
    if (nextEpisode?.id) onNavigate(nextEpisode.id);
  }, [nextEpisode, onNavigate]);

  const cancel = useCallback(() => {
    setCancelled(true);
    if (intervalRef.current) clearInterval(intervalRef.current);
  }, []);

  return {
    visible,
    ended,
    countdown,
    nextEpisode,
    playNow,
    cancel,
  };
}
