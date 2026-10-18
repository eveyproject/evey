import { useCallback, useEffect, useRef, useState } from "react";
import { CREDITS_COUNTDOWN_SEC, type ResolvedSegment } from "../lib/skipSegments";
import { noteAutoAdvance, useStillWatchingHold } from "../lib/stillWatching";

// A playhead that moves this far between two ticks was moved by the viewer:
// the ticks come twice a second, the fastest speed covers two seconds, a
// skip key or a scrub covers more.
export const CREDITS_SEEK_JUMP_SEC = 4;

// The credits countdown shared by the browser, the phone and the televisions.
// The clock starts where the viewer ENTERS the credits, not at the segment:
// measured from the segment, a resume inside credits marked four minutes early
// left six seconds before the next episode, on every return (prod 2026-10-15).
// On the playhead (a pause freezes it), never past the segment end. "Keep
// watching" (the button, Escape, Back, or a seek inside the credits) is a
// decision for these credits; a seek back must not restart it.
export function useCreditsCountdown({
  segment,
  currentTime,
  mediaId,
  enabled,
  onNextEpisode,
}: {
  /** The segment the playhead is inside; anything but end_credits is ignored. */
  segment: ResolvedSegment | null;
  currentTime: number;
  mediaId: string | undefined;
  /** The profile's auto-play choice: off, the buttons stay and nothing counts. */
  enabled: boolean;
  /** Where the countdown lands; without it there is nothing to count to. */
  onNextEpisode: (() => void) | undefined;
}) {
  const credits = segment?.type === "end_credits" && onNextEpisode ? segment : null;
  const startSec = credits?.startSec;
  const endSec = credits?.endSec;

  const [cancelled, setCancelled] = useState<Set<number>>(() => new Set());
  const [lastMedia, setLastMedia] = useState(mediaId);
  const firedRef = useRef<number | null>(null);
  if (lastMedia !== mediaId) {
    setLastMedia(mediaId);
    setCancelled(new Set());
    firedRef.current = null;
  }

  // Where the playhead was when these credits became the active segment.
  const entryRef = useRef<{ startSec: number; atSec: number } | null>(null);
  const alreadyInside = startSec !== undefined && entryRef.current?.startSec === startSec;
  if (startSec === undefined) {
    entryRef.current = null;
  } else if (!alreadyInside) {
    entryRef.current = { startSec, atSec: Math.max(startSec, currentTime) };
  }

  // A seek while the clock runs is the viewer saying they are still here.
  // Decided in render, not in an effect: the jump could land past the clock
  // and the fire effect would see zero before the cancel took.
  const prevTimeRef = useRef(currentTime);
  const jumped = alreadyInside && Math.abs(currentTime - prevTimeRef.current) > CREDITS_SEEK_JUMP_SEC;
  prevTimeRef.current = currentTime;
  const cancelledHere = startSec !== undefined && cancelled.has(startSec);
  if (jumped && enabled && startSec !== undefined && !cancelledHere) {
    setCancelled((prev) => new Set(prev).add(startSec));
  }

  // Past the "still watching?" threshold the credits wait for the viewer.
  const hold = useStillWatchingHold();
  const counting = enabled && !hold && startSec !== undefined && !cancelledHere && !jumped;
  const remaining =
    counting && endSec !== undefined && entryRef.current
      ? Math.max(0, Math.ceil(Math.min(entryRef.current.atSec + CREDITS_COUNTDOWN_SEC, endSec) - currentTime))
      : null;

  const onNextRef = useRef(onNextEpisode);
  onNextRef.current = onNextEpisode;
  useEffect(() => {
    if (remaining !== 0 || startSec === undefined || firedRef.current === startSec) return;
    firedRef.current = startSec;
    noteAutoAdvance();
    onNextRef.current?.();
  }, [remaining, startSec]);

  const watchCredits = useCallback(() => {
    if (startSec === undefined) return;
    setCancelled((prev) => new Set(prev).add(startSec));
  }, [startSec]);

  return { counting, remaining, watchCredits };
}
