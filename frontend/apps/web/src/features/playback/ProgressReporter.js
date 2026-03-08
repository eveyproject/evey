import { useEffect } from "react";
import { usePlaybackServiceReportProgress } from "@evey/api";
export function ProgressReporter({ sessionId, videoRef }) {
    const reportProgress = usePlaybackServiceReportProgress();
    useEffect(() => {
        const id = setInterval(() => {
            const video = videoRef.current;
            if (!video || video.paused)
                return;
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
