import { useEffect, useRef } from "react";
import { playbackServiceReportProgress } from "@evey/api";
import { toast } from "@evey/store";
import { logger } from "../lib/log";

const log = logger("progress");

const INTERVAL_MS = 3_000;

interface ProgressReporterCallbacks {
  onRemotePause?: () => void;
  onRemotePlay?: () => void;
  onRemoteStop?: () => void;
}

export function useProgressReporter(
  sessionId: string | null,
  getCurrentPositionMs: () => number,
  getIsPaused?: () => boolean,
  getVolume?: () => number,
  getIsMuted?: () => boolean,
  callbacks?: ProgressReporterCallbacks,
) {
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const pendingRef = useRef(false);
  const errorCountRef = useRef(0);
  const getPositionRef = useRef(getCurrentPositionMs);
  getPositionRef.current = getCurrentPositionMs;
  const getIsPausedRef = useRef(getIsPaused);
  getIsPausedRef.current = getIsPaused;
  const getVolumeRef = useRef(getVolume);
  getVolumeRef.current = getVolume;
  const getIsMutedRef = useRef(getIsMuted);
  getIsMutedRef.current = getIsMuted;
  const cbRef = useRef(callbacks);
  cbRef.current = callbacks;

  useEffect(() => {
    if (!sessionId) return;

    intervalRef.current = setInterval(async () => {
      if (pendingRef.current) return;
      if (errorCountRef.current > 0 && errorCountRef.current % 3 !== 0) {
        errorCountRef.current++;
        return;
      }
      pendingRef.current = true;
      try {
        const res = await playbackServiceReportProgress({
          sessionId,
          positionMs: String(Math.round(getPositionRef.current())),
          isPaused: getIsPausedRef.current?.() ?? false,
          isMuted: getIsMutedRef.current?.() ?? false,
          volumeLevel: Math.round((getVolumeRef.current?.() ?? 1) * 100),
        });
        errorCountRef.current = 0;
        if (res?.remoteCommand) {
          if (res.remoteCommand === "pause") {
            cbRef.current?.onRemotePause?.();
          } else if (res.remoteCommand === "play") {
            cbRef.current?.onRemotePlay?.();
          } else if (res.remoteCommand === "stop") {
            cbRef.current?.onRemotePause?.();
            cbRef.current?.onRemoteStop?.();
          } else if (res.remoteCommand.startsWith("message:")) {
            toast.info(res.remoteCommand.slice(8));
          }
        }
      } catch {
        errorCountRef.current++;
      } finally {
        pendingRef.current = false;
      }
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
      }).catch((err) => log.warn("progress report failed on unmount:", err));
    };
  }, [sessionId]);
}
