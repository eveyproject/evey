import { useCallback, useEffect, useRef, useState } from "react";

const CREDITS_THRESHOLD_SEC = 30;
export const COUNTDOWN_SEC = 5;

// The "up next" card's life: shown in the credits and at the end, counting
// down (when the profile auto-plays) to the next episode. The web's overlay
// and the shared useNextEpisode (phone, televisions) kept a copy each until
// 2026-09-24, and only the web's cleared the end state on a seek back (the
// card otherwise stayed up, counting, mid-episode) and moved on once (a new
// onPlay from the caller after the countdown navigated a second time).
export function useUpNextCountdown({
  nextEpisodeId,
  currentTime,
  duration,
  onPlay,
  autoAdvance = true,
  suppress = false,
}: {
  nextEpisodeId: string | undefined;
  currentTime: number;
  duration: number;
  onPlay: (episodeId: string) => void;
  // Per-profile preference: off, the card still appears but never counts.
  autoAdvance?: boolean;
  // Another surface (a cast session, SyncPlay) owns what plays next.
  suppress?: boolean;
}) {
  const [countdown, setCountdown] = useState(COUNTDOWN_SEC);
  const [cancelled, setCancelled] = useState(false);
  const [ended, setEnded] = useState(false);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const firedRef = useRef(false);
  const onPlayRef = useRef(onPlay);
  onPlayRef.current = onPlay;

  const isInCredits = duration > 0 && currentTime > 0 && duration - currentTime <= CREDITS_THRESHOLD_SEC;
  const videoEnded = duration > 0 && currentTime >= duration - 0.5;

  // Both ways: a seek back after the end clears the countdown.
  useEffect(() => {
    setEnded(videoEnded);
  }, [videoEnded]);

  // A new next episode is a new cycle.
  useEffect(() => {
    setCancelled(false);
    firedRef.current = false;
  }, [nextEpisodeId]);

  const visible = !!nextEpisodeId && !cancelled && !suppress && (isInCredits || ended);
  // Counting down is the auto-advance itself: without it the card is a plain
  // "up next" prompt that waits for the viewer.
  const counting = ended && autoAdvance && visible;

  useEffect(() => {
    if (!counting) return;
    setCountdown(COUNTDOWN_SEC);
    intervalRef.current = setInterval(() => setCountdown((prev) => prev - 1), 1000);
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, [counting]);

  const fire = useCallback(() => {
    if (firedRef.current || !nextEpisodeId) return;
    firedRef.current = true;
    if (intervalRef.current) clearInterval(intervalRef.current);
    onPlayRef.current(nextEpisodeId);
  }, [nextEpisodeId]);

  useEffect(() => {
    if (counting && countdown <= 0) fire();
  }, [counting, countdown, fire]);

  const cancel = useCallback(() => {
    setCancelled(true);
    if (intervalRef.current) clearInterval(intervalRef.current);
  }, []);

  return { visible, ended, counting, countdown, playNow: fire, cancel };
}
