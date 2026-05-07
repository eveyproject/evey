import { useEffect, useRef } from "react";
import { usePlaybackServiceMarkAsWatched, usePlaybackServiceGetWatchProgress } from "@evey/api";
import { useAuthStore } from "@evey/store";

interface AutoMarkWatchedProps {
  mediaId: string;
  videoRef: React.RefObject<HTMLVideoElement | null>;
  /** Optional override when the video element only sees a transcoded duration. */
  knownDurationSec?: number;
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
export function AutoMarkWatched({ mediaId, videoRef, knownDurationSec }: AutoMarkWatchedProps) {
  const profileId = useAuthStore((s) => s.profileId) ?? "";
  const { data: progress } = usePlaybackServiceGetWatchProgress(
    mediaId,
    { profileId },
    { query: { enabled: !!mediaId && !!profileId } },
  );
  const { mutate } = usePlaybackServiceMarkAsWatched();
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
      const cur = video.currentTime;
      if (cur <= 0) return;
      if (cur / dur >= THRESHOLD) {
        firedRef.current = true;
        mutate({ mediaItemId: mediaId, params: { profileId } });
      }
    };

    video.addEventListener("timeupdate", tick);
    return () => video.removeEventListener("timeupdate", tick);
  }, [mediaId, profileId, videoRef, knownDurationSec, mutate]);

  return null;
}
