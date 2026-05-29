import { useEffect, useRef } from "react";
import { playbackServiceReportProgress, eveyFetch } from "@evey/api";
import { toast } from "@evey/store";

interface ProgressReporterProps {
  sessionId: string;
  videoRef: React.RefObject<HTMLVideoElement | null>;
  transcodeOffsetRef?: React.RefObject<number>;
  /** Current media item id — drives the predictive next-episode warm-up. */
  mediaId?: string;
  /** Media type (episode / movie / ...) — gates the warm-up to episodes. */
  mediaType?: string;
  onRemoteStop?: () => void;
}

// 0.85 = trigger at ~85 % of duration. Sweet spot per Lumen / Netflix
// prefetching literature: enough head-time for ffmpeg to land a few
// segments before the autoplay handoff, late enough that we don't waste
// transcode budget on abandoned plays.
const PREWARM_PROGRESS_THRESHOLD = 0.85;

export function ProgressReporter({ sessionId, videoRef, transcodeOffsetRef, mediaId, mediaType, onRemoteStop }: ProgressReporterProps) {
  const pendingRef = useRef(false);
  const errorCountRef = useRef(0);
  // One prewarm per media item per mount — fires exactly when the
  // current play first crosses the threshold. Resets whenever the
  // mediaId changes (next ep starts).
  const prewarmedForRef = useRef<string | null>(null);
  const onRemoteStopRef = useRef(onRemoteStop);
  onRemoteStopRef.current = onRemoteStop;

  useEffect(() => {
    const id = setInterval(async () => {
      const video = videoRef.current;
      if (!video || pendingRef.current) return;
      if (errorCountRef.current > 0 && errorCountRef.current % 3 !== 0) {
        errorCountRef.current++;
        return;
      }
      pendingRef.current = true;
      try {
        const res = await playbackServiceReportProgress({
          sessionId,
          positionMs: String(Math.floor(((transcodeOffsetRef?.current ?? 0) + video.currentTime) * 1000)),
          isPaused: video.paused,
          isMuted: video.muted,
          volumeLevel: Math.round(video.volume * 100),
        });
        errorCountRef.current = 0;
        // Predictive next-episode warm-up. Only for series episodes, only
        // once per (mount, mediaId), only past the threshold. Failure is
        // fire-and-forget — the next ep autoplay still works without it.
        if (
          mediaId &&
          mediaType === "episode" &&
          prewarmedForRef.current !== mediaId &&
          video.duration > 0 &&
          video.currentTime / video.duration >= PREWARM_PROGRESS_THRESHOLD
        ) {
          prewarmedForRef.current = mediaId;
          eveyFetch<{ next_media_item_id?: string }>({
            url: `/api/v1/media/${encodeURIComponent(mediaId)}/prewarm-next`,
            method: "POST",
          }).catch((err) => {
            // 204 (series finale) lands as a no-content response and
            // eveyFetch can resolve to undefined — both are fine. Real
            // network errors get logged but never bubble.
            console.debug("prewarm-next failed (non-fatal)", err);
          });
        }
        if (res?.remoteCommand) {
          if (res.remoteCommand === "pause") {
            video.pause();
          } else if (res.remoteCommand === "play") {
            video.play().catch((err) => console.warn("video.play failed", err));
          } else if (res.remoteCommand === "stop") {
            video.pause();
            onRemoteStopRef.current?.();
          } else if (res.remoteCommand.startsWith("message:")) {
            toast.info(res.remoteCommand.slice(8));
          }
        }
      } catch (err) {
        console.warn("progress report failed", err);
        errorCountRef.current++;
      } finally {
        pendingRef.current = false;
      }
    }, 3_000);

    return () => {
      clearInterval(id);
      const video = videoRef.current;
      if (!video) return;
      playbackServiceReportProgress({
        sessionId,
        positionMs: String(Math.floor(((transcodeOffsetRef?.current ?? 0) + video.currentTime) * 1000)),
        isPaused: video.paused,
        isMuted: video.muted,
        volumeLevel: Math.round(video.volume * 100),
      }).catch((err) => console.warn("final progress report failed", err));
    };
    // Re-create the interval when the underlying mediaId changes so a new
    // ep gets its own prewarmedForRef bookkeeping.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId, videoRef, mediaId, mediaType]);

  return null;
}
