import { useEffect, useRef } from "react";
import { playbackServiceReportProgress, eveyFetch } from "@evey/api";
import { toast } from "@evey/store";

const INTERVAL_MS = 3_000;

interface ProgressReporterCallbacks {
  onRemotePause?: () => void;
  onRemotePlay?: () => void;
  onRemoteStop?: () => void;
}

// 0.85 = trigger at ~85% of duration - same threshold and route as the web
// player (ProgressReporter.tsx), giving ffmpeg a few seconds of head-time to
// land the next episode's first segments before the autoplay handoff. On the
// NAS-class hardware this project targets, a cold transcode start is exactly
// the stall this prewarm exists to avoid.
export const PREWARM_PROGRESS_THRESHOLD = 0.85;

interface ShouldTriggerPrewarmParams {
  mediaId?: string;
  isEpisode?: boolean;
  positionMs: number;
  durationMs: number;
  /** The mediaId already prewarmed for the current mount, or null. */
  prewarmedMediaId: string | null;
}

// Pure trigger rule, kept separate from the interval/fetch plumbing so it can
// be exhaustively tested without rendering a hook. Once-per-media is enforced
// by the caller comparing prewarmedMediaId against mediaId - a POST per tick
// would hammer the transcoder on weak hardware.
export function shouldTriggerPrewarm({
  mediaId,
  isEpisode,
  positionMs,
  durationMs,
  prewarmedMediaId,
}: ShouldTriggerPrewarmParams): boolean {
  if (!mediaId || !isEpisode) return false;
  if (prewarmedMediaId === mediaId) return false;
  if (durationMs <= 0) return false;
  return positionMs / durationMs >= PREWARM_PROGRESS_THRESHOLD;
}

interface PrewarmNextOptions {
  /** Current media item id - drives the predictive next-episode warm-up. */
  mediaId?: string;
  /** Gates the warm-up to series episodes, matching the web player. */
  isEpisode?: boolean;
  /** Total duration of the current item, in milliseconds. */
  getDurationMs?: () => number;
}

export function useProgressReporter(
  sessionId: string | null,
  getCurrentPositionMs: () => number,
  getIsPaused?: () => boolean,
  getVolume?: () => number,
  getIsMuted?: () => boolean,
  callbacks?: ProgressReporterCallbacks,
  prewarm?: PrewarmNextOptions,
  // Friendly name of the cast device while casting, "" for local playback.
  // Naming the device is what makes the server withhold remote commands from
  // this reporter so the TV's own beacon consumes them (admin pause/message
  // used to land on the parked phone instead of the television).
  getCastDeviceName?: () => string,
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
  const prewarmRef = useRef(prewarm);
  prewarmRef.current = prewarm;
  const getCastDeviceNameRef = useRef(getCastDeviceName);
  getCastDeviceNameRef.current = getCastDeviceName;
  const prewarmedMediaIdRef = useRef<string | null>(null);

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
          castDeviceName: getCastDeviceNameRef.current?.() ?? "",
        });
        errorCountRef.current = 0;

        const pn = prewarmRef.current;
        if (pn?.mediaId) {
          const durationMs = pn.getDurationMs?.() ?? 0;
          if (
            shouldTriggerPrewarm({
              mediaId: pn.mediaId,
              isEpisode: pn.isEpisode,
              positionMs: getPositionRef.current(),
              durationMs,
              prewarmedMediaId: prewarmedMediaIdRef.current,
            })
          ) {
            prewarmedMediaIdRef.current = pn.mediaId;
            eveyFetch<{ next_media_item_id?: string }>({
              url: `/api/v1/media/${encodeURIComponent(pn.mediaId)}/prewarm-next`,
              method: "POST",
            }).catch((err) => console.warn("prewarm-next failed (non-fatal)", err));
          }
        }

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
        castDeviceName: getCastDeviceNameRef.current?.() ?? "",
      }).catch((err) => console.warn("caught", err));
    };
  }, [sessionId]);
}
