import { useEffect, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { usePlaybackServiceGetWatchProgress, usePlaybackServiceMarkAsWatched } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { invalidateWatchedState } from "../lib/invalidateWatchedState";

/** The standard watched ceiling (MaxResumePct): past 90% of the runtime, the title is watched. */
export const AUTO_WATCHED_THRESHOLD = 0.9;

/**
 * Marks the title watched once playback crosses AUTO_WATCHED_THRESHOLD of its
 * runtime, once per title. The server's progress reports always send
 * Completed=false, so without this the viewer had to toggle "watched" by hand
 * after every episode. A title the server already knows as played is left
 * alone, and every watched-sensitive view (badges, Up Next, home feeds) is
 * refreshed once the mark lands.
 *
 * positionSec must be the ABSOLUTE playhead: a player whose element restarts
 * at 0 after a seek-restart adds its own offset first.
 */
export function useAutoMarkWatched(mediaId: string, positionSec: number, durationSec: number): void {
  const profileId = useAuthStore((s) => s.profileId) ?? "";
  const { data: progress } = usePlaybackServiceGetWatchProgress(
    mediaId,
    { profileId },
    { query: { enabled: !!mediaId && !!profileId } },
  );
  const qc = useQueryClient();
  const { mutate } = usePlaybackServiceMarkAsWatched({
    mutation: { onSuccess: () => invalidateWatchedState(qc, mediaId) },
  });
  const firedRef = useRef(false);
  useEffect(() => { firedRef.current = false; }, [mediaId]);
  useEffect(() => {
    if (firedRef.current) return;
    if (progress?.played) { firedRef.current = true; return; }
    if (!mediaId || !profileId) return;
    if (!Number.isFinite(durationSec) || durationSec <= 0 || positionSec <= 0) return;
    if (positionSec / durationSec >= AUTO_WATCHED_THRESHOLD) {
      firedRef.current = true;
      mutate({ mediaItemId: mediaId, params: { profileId } });
    }
  }, [positionSec, durationSec, progress?.played, profileId, mediaId, mutate]);
}
