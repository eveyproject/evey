import { useEffect, useRef } from "react";
import { playbackServiceReportProgress, eveyFetch } from "@evey/api";
import { toast } from "@evey/store";
import { clientLogger } from "../log";

const log = clientLogger("progress");

const INTERVAL_MS = 3_000;

// How long an admin's message stays on screen during playback: the web's
// in-player banner and the cast receiver hold it 12 s. The other players
// showed it as a plain 3 s toast, gone before a viewer on a sofa had read it.
export const ADMIN_MESSAGE_MS = 12_000;

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
  // The last position a tick read. The final report runs while the screen
  // is popped, when the native player may already be released and read as 0:
  // that 0 was the freshest write and erased the resume point (measured on the
  // TV emulator, 2026-09-25: 267 s reported, 0 in watch_progress 3 s later).
  const lastPositionRef = useRef(0);

  useEffect(() => {
    if (!sessionId) return;
    // A new session starts from zero: the previous title's last tick must not
    // be what the first final report of this one falls back to.
    lastPositionRef.current = 0;

    intervalRef.current = setInterval(async () => {
      if (pendingRef.current) return;
      if (errorCountRef.current > 0 && errorCountRef.current % 3 !== 0) {
        errorCountRef.current++;
        return;
      }
      pendingRef.current = true;
      try {
        const tickPositionMs = getPositionRef.current();
        if (tickPositionMs > 0) lastPositionRef.current = tickPositionMs;
        const res = await playbackServiceReportProgress({
          sessionId,
          positionMs: String(Math.round(tickPositionMs)),
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
            }).catch((err) => log.warn("prewarm-next failed (non-fatal)", err));
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
            toast.info(res.remoteCommand.slice(8), { duration: ADMIN_MESSAGE_MS });
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
      const read = getPositionRef.current();
      const positionMs = read > 0 ? read : lastPositionRef.current;
      playbackServiceReportProgress({
        sessionId,
        positionMs: String(Math.round(positionMs)),
        isPaused: getIsPausedRef.current?.() ?? true,
        isMuted: getIsMutedRef.current?.() ?? false,
        volumeLevel: Math.round((getVolumeRef.current?.() ?? 1) * 100),
        castDeviceName: getCastDeviceNameRef.current?.() ?? "",
      }).catch((err) => log.warn("progress report failed on unmount:", err));
    };
  }, [sessionId]);
}
