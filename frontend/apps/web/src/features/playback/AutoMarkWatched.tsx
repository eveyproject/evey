import { useEffect, useRef } from "react";
import { invalidateWatchedState } from "@/lib/invalidateWatchedState";
import { useQueryClient } from "@tanstack/react-query";
import { usePlaybackServiceMarkAsWatched, usePlaybackServiceGetWatchProgress } from "@evey/api";
import { useAuthStore } from "@evey/store";

interface AutoMarkWatchedProps {
  mediaId: string;
  videoRef: React.RefObject<HTMLVideoElement | null>;
  /** Optional override when the video element only sees a transcoded duration. */
  knownDurationSec?: number;
  /**
   * In transcode mode the `<video>` element's currentTime is local to the
   * current transcode segment (starts at 0 after every seek-restart). The
   * absolute playhead position the server cares about is `transcodeOffset
   * + video.currentTime`. Pass the offset ref so the 90% check uses the
   * absolute position; otherwise a user who seeks to 80% then plays 5min
   * never trips the threshold (5min / fullDuration is far from 90%).
   */
  transcodeOffsetRef?: React.RefObject<number>;
}

const THRESHOLD = 0.9;

/**
 * Mirror of Jellyfin's MaxResumePct=90: once playback crosses 90% of the
 * media's runtime, fire MarkAsWatched once. The server's
 * ReportProgressBySession path always sends Completed=false, so without
 * this the user has to manually toggle "watched" after every episode.
 *
 * The component is render-less; mount alongside the player.
 */
export function AutoMarkWatched({ mediaId, videoRef, knownDurationSec, transcodeOffsetRef }: AutoMarkWatchedProps) {
  const profileId = useAuthStore((s) => s.profileId) ?? "";
  const { data: progress } = usePlaybackServiceGetWatchProgress(
    mediaId,
    { profileId },
    { query: { enabled: !!mediaId && !!profileId } },
  );
  const queryClient = useQueryClient();
  const { mutate } = usePlaybackServiceMarkAsWatched({
    mutation: {
      // Mirror the manual mark-watched paths (MediaCard.invalidateItem): without
      // this the silent 90% auto-mark updated the server but no cache, so a
      // finished episode lingered as unwatched / in "Continue Watching" until
      // staleTime (5min) elapsed.
      onSuccess: () => {
        invalidateWatchedState(queryClient, mediaId);
      },
    },
  });
  const firedRef = useRef(false);

  // Reset on media change.
  useEffect(() => { firedRef.current = false; }, [mediaId]);

  // Pre-flag if the server already says it's played, to avoid redundant calls.
  useEffect(() => {
    if (progress?.played) firedRef.current = true;
  }, [progress?.played]);

  useEffect(() => {
    if (!mediaId || !profileId) return;
    const video = videoRef.current;
    if (!video) return;

    const tick = () => {
      if (firedRef.current) return;
      const dur = knownDurationSec && knownDurationSec > 0 ? knownDurationSec : video.duration;
      if (!Number.isFinite(dur) || dur <= 0) return;
      const cur = (transcodeOffsetRef?.current ?? 0) + video.currentTime;
      if (cur <= 0) return;
      if (cur / dur >= THRESHOLD) {
        firedRef.current = true;
        mutate({ mediaItemId: mediaId, params: { profileId } });
      }
    };

    video.addEventListener("timeupdate", tick);
    return () => video.removeEventListener("timeupdate", tick);
  }, [mediaId, profileId, videoRef, knownDurationSec, transcodeOffsetRef, mutate]);

  return null;
}
