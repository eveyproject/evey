import { useEffect, useRef } from "react";
import { playbackServiceReportProgress } from "@evey/api";

const INTERVAL_MS = 10_000;

export function useProgressReporter(
  sessionId: string | null,
  getCurrentPositionMs: () => number,
) {
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    if (!sessionId) return;

    intervalRef.current = setInterval(() => {
      const positionMs = getCurrentPositionMs();
      playbackServiceReportProgress({
        sessionId,
        positionMs: String(Math.round(positionMs)),
        isPaused: false,
      }).catch((err) => console.warn("progress report failed:", err));
    }, INTERVAL_MS);

    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, [sessionId, getCurrentPositionMs]);
}
