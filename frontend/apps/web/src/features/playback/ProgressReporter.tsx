import { useEffect, useRef } from "react";
import { usePlaybackServiceReportProgress } from "@evey/api";

interface ProgressReporterProps {
  sessionId: string;
  videoRef: React.RefObject<HTMLVideoElement | null>;
  transcodeOffsetRef?: React.RefObject<number>;
}

export function ProgressReporter({ sessionId, videoRef, transcodeOffsetRef }: ProgressReporterProps) {
  const reportProgress = usePlaybackServiceReportProgress();
  const reportRef = useRef(reportProgress);
  reportRef.current = reportProgress;
  // Track consecutive errors to implement a simple backoff
  const errorCountRef = useRef(0);

  useEffect(() => {
    const id = setInterval(() => {
      const video = videoRef.current;
      if (!video || video.paused) return;
      if (reportRef.current.isPending) return;
      // Back off after repeated failures — retry every 3rd tick
      if (reportRef.current.isError) {
        errorCountRef.current++;
        if (errorCountRef.current % 3 !== 0) return;
      } else {
        errorCountRef.current = 0;
      }
      reportRef.current.mutate({
        data: {
          sessionId,
          positionMs: String(Math.floor(((transcodeOffsetRef?.current ?? 0) + video.currentTime) * 1000)),
          isPaused: false,
          isMuted: video.muted,
          volumeLevel: Math.round(video.volume * 100),
        },
      });
    }, 10_000);

    return () => {
      clearInterval(id);
      const video = videoRef.current;
      if (!video) return;
      reportRef.current.mutate({
        data: {
          sessionId,
          positionMs: String(Math.floor(((transcodeOffsetRef?.current ?? 0) + video.currentTime) * 1000)),
          isPaused: video.paused,
          isMuted: video.muted,
          volumeLevel: Math.round(video.volume * 100),
        },
      });
    };
  }, [sessionId, videoRef]);

  return null;
}
