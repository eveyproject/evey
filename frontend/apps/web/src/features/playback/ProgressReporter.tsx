import { useEffect, useRef } from "react";
import { usePlaybackServiceReportProgress } from "@evey/api";

interface ProgressReporterProps {
  sessionId: string;
  videoRef: React.RefObject<HTMLVideoElement | null>;
}

export function ProgressReporter({ sessionId, videoRef }: ProgressReporterProps) {
  const reportProgress = usePlaybackServiceReportProgress();
  const reportRef = useRef(reportProgress);
  reportRef.current = reportProgress;

  useEffect(() => {
    const id = setInterval(() => {
      const video = videoRef.current;
      if (!video || video.paused) return;
      reportRef.current.mutate({
        data: {
          sessionId,
          positionMs: String(Math.floor(video.currentTime * 1000)),
          isPaused: false,
          isMuted: video.muted,
          volumeLevel: Math.round(video.volume * 100),
        },
      });
    }, 10_000);

    return () => clearInterval(id);
  }, [sessionId, videoRef]);

  return null;
}
