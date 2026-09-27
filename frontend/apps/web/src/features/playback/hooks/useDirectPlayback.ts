import { useEffect } from "react";
import type { V1StreamInfo } from "@evey/api";

interface AudioTrack {
  id: string;
  enabled: boolean;
}

interface AudioTrackList {
  length: number;
  [index: number]: AudioTrack;
}

interface HTMLVideoElementWithAudioTracks extends HTMLVideoElement {
  audioTracks?: AudioTrackList;
}

// Progressive direct-play only. The HLS pipeline (transcode AND direct_stream
// container remux) lives entirely in useTranscodePlayback - this hook just
// points the <video> element at the original file's URL and handles token
// refresh / fallback-to-transcode on error.
interface UseDirectPlaybackArgs {
  videoRef: React.RefObject<HTMLVideoElement | null>;
  positionRef: React.MutableRefObject<number>;
  streamInfo: V1StreamInfo;
  startPositionMs: number;
  transcodeMode: boolean;
  onEnded?: () => void;
  setTranscodeMode: (v: boolean) => void;
  setIsBuffering: (v: boolean) => void;
  streamTokenRef?: React.MutableRefObject<string>;
  onStreamTokenRefreshRef?: React.MutableRefObject<() => Promise<string>>;
}

function replaceTokenInUrl(url: string, newToken: string): string | null {
  try {
    const u = new URL(url, window.location.origin);
    if (!u.searchParams.has("token")) return null;
    u.searchParams.set("token", newToken);
    return u.pathname + (u.search ? u.search : "") + (u.hash ?? "");
  } catch {
    return null;
  }
}

export function useDirectPlayback({
  videoRef,
  positionRef,
  streamInfo,
  startPositionMs,
  transcodeMode,
  onEnded,
  setTranscodeMode,
  setIsBuffering,
  streamTokenRef,
  onStreamTokenRefreshRef,
}: UseDirectPlaybackArgs) {
  useEffect(() => {
    const video = videoRef.current;
    if (!video || transcodeMode) return;

    if (streamInfo.directPlayUrl) {
      video.src = streamInfo.directPlayUrl;
      video.currentTime = startPositionMs / 1000;
      video.play().catch((err) => console.warn("video.play failed", err));
    } else {
      // Server said DIRECT_PLAY but provided no URL - fall back to transcode
      // rather than leaving the <video> unloaded with no visible error.
      setTranscodeMode(true);
      return;
    }

    let canPlayTimer: ReturnType<typeof setTimeout> | null = null;
    function handleCanPlay() {
      canPlayTimer = setTimeout(() => {
        const v = videoRef.current as HTMLVideoElementWithAudioTracks | null;
        if (!v || transcodeMode) return;
        const tracks = v.audioTracks;
        if (tracks && tracks.length === 0) setTranscodeMode(true);
      }, 3000);
    }
    function handleEnded() { onEnded?.(); }
    let tokenRefreshAttempted = false;
    function handleError() {
      const v = video;
      if (!v) return;
      const e = v.error;
      if (!e) return;
      if (import.meta.env.DEV) console.warn("[player] direct play error:", e.code, e.message);
      // The most common error after a long pause is a 401 from the stream token
      // expiring (5-min sliding TTL). The HTML5 video element exposes only a
      // generic MediaError, so we can't read the HTTP status - but we can try
      // refreshing the token and retrying the same URL once before falling
      // back to a transcode mode switch (which is heavier and re-encodes from
      // the current position).
      if (
        !tokenRefreshAttempted &&
        onStreamTokenRefreshRef?.current &&
        streamTokenRef &&
        v.src &&
        v.src.includes("token=")
      ) {
        tokenRefreshAttempted = true;
        const positionSec = positionRef.current || (v.currentTime || 0);
        (async () => {
          try {
            const newToken = await onStreamTokenRefreshRef.current();
            streamTokenRef.current = newToken;
            const nextSrc = replaceTokenInUrl(v.src, newToken);
            if (!nextSrc) {
              setTranscodeMode(true);
              return;
            }
            v.src = nextSrc;
            v.currentTime = positionSec;
            try { await v.play(); } catch (err) { console.warn("video.play after token refresh failed", err); }
          } catch {
            setTranscodeMode(true);
          }
        })();
        return;
      }
      setTranscodeMode(true);
    }
    function handleTimeUpdate() { positionRef.current = video!.currentTime; }
    // A PAUSED player is not buffering - see useTranscodePlayback: the browser
    // fires `waiting` on pause too, with no `playing` to clear it.
    function handleWaiting() { if (!video!.paused) setIsBuffering(true); }
    function handlePlaying() { setIsBuffering(false); }
    video.addEventListener("timeupdate", handleTimeUpdate);
    video.addEventListener("ended", handleEnded);
    video.addEventListener("canplay", handleCanPlay);
    video.addEventListener("error", handleError);
    video.addEventListener("waiting", handleWaiting);
    video.addEventListener("playing", handlePlaying);
    video.addEventListener("canplaythrough", handlePlaying);
    video.addEventListener("pause", handlePlaying);

    return () => {
      if (canPlayTimer !== null) clearTimeout(canPlayTimer);
      video.removeEventListener("timeupdate", handleTimeUpdate);
      video.removeEventListener("ended", handleEnded);
      video.removeEventListener("canplay", handleCanPlay);
      video.removeEventListener("error", handleError);
      video.removeEventListener("waiting", handleWaiting);
      video.removeEventListener("playing", handlePlaying);
      video.removeEventListener("canplaythrough", handlePlaying);
      video.removeEventListener("pause", handlePlaying);
    };
  // The refs and setters are stable at the call site (useRef / useState /
  // useCallback with no deps), so listing them never re-runs the effect.
  }, [streamInfo, startPositionMs, onEnded, transcodeMode, videoRef, positionRef, setTranscodeMode, setIsBuffering, streamTokenRef, onStreamTokenRefreshRef]);
}
