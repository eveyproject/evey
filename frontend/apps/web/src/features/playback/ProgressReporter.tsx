import { useEffect, useRef } from "react";
import { playbackServiceReportProgress } from "@evey/api";
import { toast } from "@evey/store";

interface ProgressReporterProps {
  sessionId: string;
  videoRef: React.RefObject<HTMLVideoElement | null>;
  transcodeOffsetRef?: React.RefObject<number>;
  onRemoteStop?: () => void;
}

export function ProgressReporter({ sessionId, videoRef, transcodeOffsetRef, onRemoteStop }: ProgressReporterProps) {
  const pendingRef = useRef(false);
  const errorCountRef = useRef(0);

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
        if (res?.remoteCommand) {
          if (res.remoteCommand === "pause") {
            video.pause();
          } else if (res.remoteCommand === "play") {
            video.play().catch(() => {});
          } else if (res.remoteCommand === "stop") {
            video.pause();
            onRemoteStop?.();
          } else if (res.remoteCommand.startsWith("message:")) {
            toast.info(res.remoteCommand.slice(8));
          }
        }
      } catch {
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
      }).catch(() => {});
    };
  }, [sessionId, videoRef]);

  return null;
}
