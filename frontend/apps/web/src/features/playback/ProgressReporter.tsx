import { useEffect } from "react";
import { usePlaybackServiceReportProgress } from "@evey/api";

interface ProgressReporterProps {
  sessionId: string;
  videoRef: React.RefObject<HTMLVideoElement | null>;
}

export function ProgressReporter({ sessionId, videoRef }: ProgressReporterProps) {
  const reportProgress = usePlaybackServiceReportProgress();

  useEffect(() => {
    const id = setInterval(() => {
      const video = videoRef.current;
      if (!video || video.paused) return;
      reportProgress.mutate({
        data: {
          sessionId,
          positionMs: String(Math.floor(video.currentTime * 1000)),
          isPaused: false,
        },
      });
    }, 10_000);

    return () => clearInterval(id);
  }, [sessionId, videoRef, reportProgress]);

  return null;
}
