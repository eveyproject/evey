import { useEffect, useRef } from "react";
import { playbackServiceReportProgress } from "@evey/api";

const INTERVAL_MS = 10_000;

export function useProgressReporter(
  sessionId: string | null,
  getCurrentPositionMs: () => number,
  getIsPaused?: () => boolean,
  getVolume?: () => number,
  getIsMuted?: () => boolean,
) {
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const getPositionRef = useRef(getCurrentPositionMs);
  getPositionRef.current = getCurrentPositionMs;
  const getIsPausedRef = useRef(getIsPaused);
  getIsPausedRef.current = getIsPaused;
  const getVolumeRef = useRef(getVolume);
  getVolumeRef.current = getVolume;
  const getIsMutedRef = useRef(getIsMuted);
  getIsMutedRef.current = getIsMuted;

  useEffect(() => {
    if (!sessionId) return;

    intervalRef.current = setInterval(() => {
      const positionMs = getPositionRef.current();
      const paused = getIsPausedRef.current?.() ?? false;
      if (paused) return;
      playbackServiceReportProgress({
        sessionId,
        positionMs: String(Math.round(positionMs)),
        isPaused: false,
        isMuted: getIsMutedRef.current?.() ?? false,
        volumeLevel: Math.round((getVolumeRef.current?.() ?? 1) * 100),
      }).catch((err) => console.warn("progress report failed:", err));
    }, INTERVAL_MS);

    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current);
      const positionMs = getPositionRef.current();
      playbackServiceReportProgress({
        sessionId,
        positionMs: String(Math.round(positionMs)),
        isPaused: getIsPausedRef.current?.() ?? true,
        isMuted: getIsMutedRef.current?.() ?? false,
        volumeLevel: Math.round((getVolumeRef.current?.() ?? 1) * 100),
      }).catch((err) => console.warn("final progress report failed:", err));
    };
  }, [sessionId]);
}
