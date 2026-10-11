import { useEffect, useRef } from "react";
import type { TrackPick } from "@evey/api";
import { clearPlaying, markPlaying } from "../lib/interruptedPlayback";

const MARK_EVERY_MS = 10_000;

/**
 * Keeps the "this title is playing" mark the next launch reads (see
 * lib/interruptedPlayback.ts): written as soon as playback runs, then every
 * ten seconds, wiped when the player is left. Off when `enabled` is false (a
 * watch-together room, whose title is the host's to restart).
 */
export function useInterruptedPlaybackMarker(opts: {
  enabled: boolean;
  mediaId: string;
  profileId: string | null | undefined;
  title: string;
  isPlaying: boolean;
  getPositionMs: () => number;
  getPick: () => TrackPick;
}): void {
  const { enabled, mediaId, profileId, isPlaying } = opts;
  // The getters and the title change on every render; the timer reads them here.
  const latest = useRef(opts);
  latest.current = opts;

  useEffect(() => {
    if (!enabled || !mediaId || !profileId || !isPlaying) return;
    const write = () => {
      const o = latest.current;
      void markPlaying({
        mediaId,
        profileId,
        title: o.title,
        positionMs: Math.max(0, Math.round(o.getPositionMs())),
        pick: o.getPick(),
        at: Date.now(),
      });
    };
    write();
    const timer = setInterval(write, MARK_EVERY_MS);
    return () => clearInterval(timer);
  }, [enabled, mediaId, profileId, isPlaying]);

  // Leaving the player is the viewer's own choice: nothing to offer back.
  useEffect(() => {
    if (!enabled) return;
    return () => { void clearPlaying(); };
  }, [enabled, mediaId]);
}
