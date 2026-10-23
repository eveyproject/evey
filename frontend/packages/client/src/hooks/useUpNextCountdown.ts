import { useCallback, useEffect, useRef, useState } from "react";
import { noteAutoAdvance, STILL_WATCHING_EXIT_MS, useStillWatchingHold } from "../lib/stillWatching";

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
  onStillWatchingTimeout,
}: {
  nextEpisodeId: string | undefined;
  currentTime: number;
  duration: number;
  onPlay: (episodeId: string) => void;
  // Per-profile preference: off, the card still appears but never counts.
  autoAdvance?: boolean;
  // Another surface (a cast session, SyncPlay) owns what plays next.
  suppress?: boolean;
  // Nobody answered the "still watching?" card: the player leaves (stillWatching.ts).
  onStillWatchingTimeout?: () => void;
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
  // "up next" prompt that waits for the viewer. Past the "still watching?"
  // threshold it waits too, until the viewer does something (stillWatching.ts).
  const hold = useStillWatchingHold();
  const counting = ended && autoAdvance && visible && !hold;
  const stillWatching = ended && autoAdvance && visible && hold;

  useEffect(() => {
    if (!counting) return;
    setCountdown(COUNTDOWN_SEC);
    intervalRef.current = setInterval(() => setCountdown((prev) => prev - 1), 1000);
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, [counting]);

  // Left up with nobody there, the card kept the player and its session alive
  // until the television slept on its own (prod 2026-10-23). Past the wait the
  // player leaves, as at the end of a film; the next episode stays first in
  // "Continue watching".
  const timeoutRef = useRef(onStillWatchingTimeout);
  timeoutRef.current = onStillWatchingTimeout;
  useEffect(() => {
    if (!stillWatching) return;
    const t = setTimeout(() => timeoutRef.current?.(), STILL_WATCHING_EXIT_MS);
    return () => clearTimeout(t);
  }, [stillWatching]);

  const fire = useCallback(() => {
    if (firedRef.current || !nextEpisodeId) return;
    firedRef.current = true;
    if (intervalRef.current) clearInterval(intervalRef.current);
    onPlayRef.current(nextEpisodeId);
  }, [nextEpisodeId]);

  useEffect(() => {
    if (counting && countdown <= 0) {
      noteAutoAdvance();
      fire();
    }
  }, [counting, countdown, fire]);

  // No viewer activity noted here: the credits countdown calls playNow by
  // itself. The players note the real input (a click, a key, a touch).
  const cancel = useCallback(() => {
    setCancelled(true);
    if (intervalRef.current) clearInterval(intervalRef.current);
  }, []);

  return { visible, ended, counting, countdown, stillWatching, playNow: fire, cancel };
}
