import { useEffect, useRef } from "react";
import { playbackServiceReportProgress, eveyFetch } from "@evey/api";
import { toast } from "@evey/store";
import { PREWARM_PROGRESS_THRESHOLD } from "@evey/client";

/**
 * Live cast (Chromecast) playback state, relayed by the sender page. While
 * `active`, the LOCAL video is paused on the handoff frame and the remote
 * device is the real playback: progress reports must describe IT, or the
 * user's "continue watching" position freezes where the cast started and the
 * admin panel shows a paused browser while the film plays on the TV.
 */
interface CastProgressSource {
  active: boolean;
  /** Absolute position on the media timeline, seconds. */
  positionSec: number;
  isPaused: boolean;
  /** 0..1, the receiver's volume. */
  volume: number;
  isMuted: boolean;
  /** Friendly name of the cast device, persisted on the session for admins. */
  deviceName: string;
  playPause: () => void;
  stop: () => void;
}

interface ProgressReporterProps {
  sessionId: string;
  videoRef: React.RefObject<HTMLVideoElement | null>;
  transcodeOffsetRef?: React.RefObject<number>;
  /** Current media item id - drives the predictive next-episode warm-up. */
  mediaId?: string;
  /** Media type (episode / movie / ...) - gates the warm-up to episodes. */
  mediaType?: string;
  onRemoteStop?: () => void;
  /**
   * Admin `message:` command. The player draws it INSIDE its container: a
   * global toast lives outside the fullscreen element, which the browser
   * never paints while the player is fullscreen. Falls back to a toast.
   */
  onRemoteMessage?: (message: string) => void;
  /** Present and active while a cast session plays this media. */
  cast?: CastProgressSource;
  /** The picture is on another device, whether or not `cast` carries a live
   *  state feed (a Remote Playback fling carries none). An admin "play" must
   *  not wake the local element in that state. */
  remoteActive?: boolean;
}

// 0.85 = trigger at ~85 % of duration. Sweet spot per streaming-service
// prefetching literature: enough head-time for ffmpeg to land a few
// segments before the autoplay handoff, late enough that we don't waste
// transcode budget on abandoned plays.

// HTMLMediaElement.HAVE_METADATA, spelled out: not every DOM implementation
// carries the constant (happy-dom does not).
const HAVE_METADATA = 1;

export function ProgressReporter({ sessionId, videoRef, transcodeOffsetRef, mediaId, mediaType, onRemoteStop, onRemoteMessage, cast, remoteActive = false }: ProgressReporterProps) {
  const pendingRef = useRef(false);
  const errorCountRef = useRef(0);
  // One prewarm per media item per mount - fires exactly when the
  // current play first crosses the threshold. Resets whenever the
  // mediaId changes (next ep starts).
  const prewarmedForRef = useRef<string | null>(null);
  const onRemoteStopRef = useRef(onRemoteStop);
  onRemoteStopRef.current = onRemoteStop;
  const onRemoteMessageRef = useRef(onRemoteMessage);
  onRemoteMessageRef.current = onRemoteMessage;
  // Mirrored into a ref so the 3s interval closure always reads the LIVE cast
  // state (the effect below deliberately doesn't re-run on every cast tick).
  const castRef = useRef(cast);
  castRef.current = cast;
  const remoteActiveRef = useRef(remoteActive);
  remoteActiveRef.current = remoteActive;

  useEffect(() => {
    // The last local state seen while the element still held this session's
    // media: at unmount videoRef is already null and a detached element reads
    // 0, so the final report is built from this, never from the live element.
    let last: { positionMs: string; isPaused: boolean; isMuted: boolean; volumeLevel: number } | null = null;
    const snapshot = (video: HTMLVideoElement) => {
      if (video.readyState < HAVE_METADATA) return;
      last = {
        positionMs: String(Math.floor(((transcodeOffsetRef?.current ?? 0) + video.currentTime) * 1000)),
        isPaused: video.paused,
        isMuted: video.muted,
        volumeLevel: Math.round(video.volume * 100),
      };
    };
    const element = videoRef.current;
    const onMediaEvent = () => { if (element) snapshot(element); };
    element?.addEventListener("timeupdate", onMediaEvent);
    element?.addEventListener("pause", onMediaEvent);
    element?.addEventListener("volumechange", onMediaEvent);

    const id = setInterval(async () => {
      const video = videoRef.current;
      const cast = castRef.current;
      const casting = cast?.active === true;
      if (video) snapshot(video);
      if (!video || pendingRef.current) return;
      if (errorCountRef.current > 0 && errorCountRef.current % 3 !== 0) {
        errorCountRef.current++;
        return;
      }
      pendingRef.current = true;
      try {
        // While casting, the report describes the REMOTE playback; the empty
        // castDeviceName of a local report clears a finished relay server-side.
        const res = await playbackServiceReportProgress(
          casting
            ? {
                sessionId,
                positionMs: String(Math.floor(cast.positionSec * 1000)),
                isPaused: cast.isPaused,
                isMuted: cast.isMuted,
                volumeLevel: Math.round(cast.volume * 100),
                castDeviceName: cast.deviceName,
              }
            : {
                sessionId,
                positionMs: String(Math.floor(((transcodeOffsetRef?.current ?? 0) + video.currentTime) * 1000)),
                isPaused: video.paused,
                isMuted: video.muted,
                volumeLevel: Math.round(video.volume * 100),
                castDeviceName: "",
              },
        );
        errorCountRef.current = 0;
        // Predictive next-episode warm-up. Only for series episodes, only
        // once per (mount, mediaId), only past the threshold, and only for
        // LOCAL playback (a cast reload swaps the whole receiver media -
        // the local autoplay chain this warm-up feeds doesn't run there).
        if (
          !casting &&
          mediaId &&
          mediaType === "episode" &&
          prewarmedForRef.current !== mediaId &&
          video.duration > 0 &&
          video.currentTime / video.duration >= PREWARM_PROGRESS_THRESHOLD
        ) {
          prewarmedForRef.current = mediaId;
          eveyFetch<{ next_media_item_id?: string }>({
            url: `/api/v1/media/${encodeURIComponent(mediaId)}/prewarm-next`,
            method: "POST",
          }).catch((err) => {
            // 204 (series finale) lands as a no-content response and
            // eveyFetch can resolve to undefined - both are fine. Real
            // network errors get logged but never bubble.
            console.debug("prewarm-next failed (non-fatal)", err);
          });
        }
        if (res?.remoteCommand) {
          // Admin remote commands drive whatever ACTUALLY plays: the cast
          // receiver while casting, the local <video> otherwise.
          if (res.remoteCommand === "pause") {
            if (casting) {
              if (!cast.isPaused) cast.playPause();
            } else {
              video.pause();
            }
          } else if (res.remoteCommand === "play") {
            if (casting) {
              if (cast.isPaused) cast.playPause();
            } else if (!remoteActiveRef.current) {
              // Only when nothing else holds the picture: a fling reports no
              // state (casting === false) yet owns the playback, and waking
              // the local element there plays the film twice.
              video.play().catch((err) => console.warn("video.play failed", err));
            }
          } else if (res.remoteCommand === "stop") {
            if (casting) {
              cast.stop();
            } else {
              video.pause();
            }
            onRemoteStopRef.current?.();
          } else if (res.remoteCommand.startsWith("message:")) {
            const message = res.remoteCommand.slice(8);
            if (onRemoteMessageRef.current) onRemoteMessageRef.current(message);
            else toast.info(message);
          }
        }
      } catch (err) {
        console.warn("progress report failed", err);
        errorCountRef.current++;
      } finally {
        pendingRef.current = false;
      }
    }, 3_000);

    return () => {
      clearInterval(id);
      element?.removeEventListener("timeupdate", onMediaEvent);
      element?.removeEventListener("pause", onMediaEvent);
      element?.removeEventListener("volumechange", onMediaEvent);
      const cast = castRef.current;
      if (cast?.active) {
        // Teardown mid-cast: persist the REMOTE position, not the stale
        // local frame the video paused on at handoff.
        playbackServiceReportProgress({
          sessionId,
          positionMs: String(Math.floor(cast.positionSec * 1000)),
          isPaused: cast.isPaused,
          isMuted: cast.isMuted,
          volumeLevel: Math.round(cast.volume * 100),
          castDeviceName: cast.deviceName,
        }).catch((err) => console.warn("final progress report failed", err));
        return;
      }
      if (!last) return;
      playbackServiceReportProgress({ sessionId, ...last, castDeviceName: "" })
        .catch((err) => console.warn("final progress report failed", err));
    };
    // Re-create the interval when the underlying mediaId changes so a new
    // ep gets its own prewarmedForRef bookkeeping.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId, videoRef, mediaId, mediaType]);

  return null;
}
