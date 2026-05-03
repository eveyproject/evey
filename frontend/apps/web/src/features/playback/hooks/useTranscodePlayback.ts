import { useEffect, useState } from "react";
import Hls from "hls.js";
import { buildTranscodeHlsUrl } from "./hlsPlayerUtils";

interface UseTranscodePlaybackArgs {
  videoRef: React.RefObject<HTMLVideoElement | null>;
  hlsRef: React.MutableRefObject<InstanceType<typeof Hls> | null>;
  positionRef: React.MutableRefObject<number>;
  transcodeOffsetRef: React.MutableRefObject<number>;
  wasPausedRef: React.MutableRefObject<boolean>;
  streamTokenRef: React.MutableRefObject<string>;
  onStreamTokenRefreshRef: React.MutableRefObject<() => Promise<string>>;
  transcodeMode: boolean;
  mediaId: string;
  currentTrackIndex: number;
  selectedBurnInTrack: number | null;
  startPositionMs: number;
  onEnded?: () => void;
  setIsBuffering: (v: boolean) => void;
  setFatalError: (v: boolean) => void;
}

export function useTranscodePlayback({
  videoRef,
  hlsRef,
  positionRef,
  transcodeOffsetRef,
  wasPausedRef,
  streamTokenRef,
  onStreamTokenRefreshRef,
  transcodeMode,
  mediaId,
  currentTrackIndex,
  selectedBurnInTrack,
  startPositionMs,
  onEnded,
  setIsBuffering,
  setFatalError,
}: UseTranscodePlaybackArgs) {
  const [transcodeStartTime, setTranscodeStartTime] = useState(startPositionMs / 1000);
  const [hlsLevels, setHlsLevels] = useState<number[]>([]);

  useEffect(() => {
    if (!transcodeMode) return;
    const video = videoRef.current;
    if (!video) return;

    if (hlsRef.current) {
      hlsRef.current.destroy();
      hlsRef.current = null;
    }

    const offset = transcodeStartTime;
    transcodeOffsetRef.current = offset > 0 ? offset : 0;

    const hlsUrl = buildTranscodeHlsUrl(
      mediaId,
      streamTokenRef.current,
      currentTrackIndex,
      selectedBurnInTrack,
      offset,
    );

    if (!Hls.isSupported()) {
      video.src = hlsUrl;
      video.currentTime = 0;
      if (!wasPausedRef.current) {
        video.play().catch(() => {
          video.muted = true;
          void video.play().catch(() => { /* user gesture required */ });
        });
      }
      function onTimeUpdate() { positionRef.current = video!.currentTime; }
      function handleEnded() { onEnded?.(); }
      video.addEventListener("timeupdate", onTimeUpdate);
      video.addEventListener("ended", handleEnded);
      return () => {
        wasPausedRef.current = video.paused;
        video.removeEventListener("timeupdate", onTimeUpdate);
        video.removeEventListener("ended", handleEnded);
        video.src = "";
      };
    }

    const hls = new Hls({
      // 0 = start at first frame of first segment. ffmpeg ships
      // -copyts -avoid_negative_ts disabled -start_at_zero on the
      // server side (Jellyfin's recipe), which translates the segment
      // PTS so the first frame lands exactly at t=0. Forcing 0 here
      // matches that, no keyframe-snap nudge.
      startPosition: 0,
      maxBufferLength: 30,
      maxMaxBufferLength: 60,
      xhrSetup: (xhr: XMLHttpRequest, url: string) => {
        const tok = streamTokenRef.current;
        if (!tok) return;
        // The server bakes the stream token into segment URIs in the master/sub
        // playlists. The token has a 5-min sliding TTL that lapses during pause,
        // so on resume those baked-in URLs return 401. After a 401 retry refreshes
        // the token, every subsequent xhr must override the URL token — so we
        // always strip and re-inject, regardless of what the URL carries.
        try {
          const u = new URL(url, window.location.origin);
          u.searchParams.set("token", tok);
          xhr.open("GET", u.toString(), true);
        } catch {
          if (!url.includes("token=")) {
            const sep = url.includes("?") ? "&" : "?";
            xhr.open("GET", `${url}${sep}token=${tok}`, true);
          }
        }
      },
    });
    hlsRef.current = hls;

    hls.loadSource(hlsUrl);
    hls.attachMedia(video);

    hls.on(Hls.Events.MANIFEST_PARSED, () => {
      setHlsLevels(hls.levels.map((l) => l.height));
      // Browsers block unmuted autoplay until the user has interacted with
      // the page. Fall back to a muted start so playback begins; the player
      // UI surfaces the muted state and the user unmutes with one click.
      video.play().catch(() => {
        video.muted = true;
        void video.play().catch(() => { /* user gesture required */ });
      });
    });

    let retries = 0;
    let consecutive404 = 0;
    hls.on(Hls.Events.ERROR, (_, data) => {
      if (!data.fatal) return;
      if (import.meta.env.DEV) console.warn("[player] HLS fatal error:", data.type, data.details, data.response?.code);
      retries++;
      if (retries > 20) {
        setFatalError(true);
        return;
      }
      if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
        if (data.response?.code === 401) {
          (async () => {
            try {
              const newToken = await onStreamTokenRefreshRef.current();
              streamTokenRef.current = newToken;
              // For manifest/level failures, hls.js holds a stale URL with the
              // expired token. xhrSetup rewrites the token on every request,
              // but loadSource is needed to retry the manifest itself; for
              // fragment failures startLoad re-drives the loader.
              const isManifestLevel =
                data.details === "manifestLoadError" ||
                data.details === "manifestLoadTimeOut" ||
                data.details === "manifestParsingError" ||
                data.details === "levelLoadError" ||
                data.details === "levelLoadTimeOut";
              if (isManifestLevel) {
                const seekPos = transcodeOffsetRef.current + (video.currentTime || 0);
                const freshUrl = buildTranscodeHlsUrl(mediaId, newToken, currentTrackIndex, selectedBurnInTrack, seekPos);
                hls.loadSource(freshUrl);
                hls.startLoad();
              } else {
                hls.startLoad();
              }
            } catch {
              setFatalError(true);
            }
          })();
          return;
        }
        if (data.response?.code === 404) {
          consecutive404++;
          if (consecutive404 >= 2) {
            hls.destroy();
            video.removeAttribute("src");
            video.load();
            setFatalError(true);
            return;
          }
          const seekPos = transcodeOffsetRef.current + (video.currentTime || 0);
          if (seekPos > 0) {
            setTranscodeStartTime(seekPos);
          }
          return;
        }
        const delay = Math.min(2000 * Math.pow(1.5, retries - 1), 15000);
        setTimeout(() => hls.startLoad(), delay);
      } else if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
        hls.recoverMediaError();
      } else {
        setFatalError(true);
      }
    });
    hls.on(Hls.Events.FRAG_LOADED, () => { retries = 0; consecutive404 = 0; });

    function onTimeUpdate() { positionRef.current = transcodeOffsetRef.current + video!.currentTime; }
    function handleEnded() { onEnded?.(); }
    function handleWaiting() { setIsBuffering(true); }
    function handlePlaying() { setIsBuffering(false); }
    video.addEventListener("timeupdate", onTimeUpdate);
    video.addEventListener("ended", handleEnded);
    video.addEventListener("waiting", handleWaiting);
    video.addEventListener("playing", handlePlaying);

    return () => {
      wasPausedRef.current = video.paused;
      video.removeEventListener("timeupdate", onTimeUpdate);
      video.removeEventListener("ended", handleEnded);
      video.removeEventListener("waiting", handleWaiting);
      video.removeEventListener("playing", handlePlaying);
      hls.destroy();
      hlsRef.current = null;
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps -- streamTokenRef/onStreamTokenRefreshRef read imperatively; setFatalError/setIsBuffering/setTranscodeStartTime are stable setState
  }, [transcodeMode, mediaId, onEnded, currentTrackIndex, transcodeStartTime, selectedBurnInTrack]);

  return { transcodeStartTime, setTranscodeStartTime, hlsLevels };
}
