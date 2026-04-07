import { useEffect, useState } from "react";
import Hls from "hls.js";
import { buildTranscodeHlsUrl } from "./hlsPlayerUtils";

interface UseTranscodePlaybackArgs {
  videoRef: React.RefObject<HTMLVideoElement | null>;
  hlsRef: React.MutableRefObject<InstanceType<typeof Hls> | null>;
  hlsLevelsRef: React.MutableRefObject<{ height: number }[]>;
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
  hlsLevelsRef,
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
      if (!wasPausedRef.current) video.play().catch(() => {});
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
      startPosition: 0,
      maxBufferLength: 30,
      maxMaxBufferLength: 60,
      xhrSetup: (xhr: XMLHttpRequest, url: string) => {
        const tok = streamTokenRef.current;
        if (tok && !url.includes("token=")) {
          const sep = url.includes("?") ? "&" : "?";
          xhr.open("GET", `${url}${sep}token=${tok}`, true);
        }
      },
    });
    hlsRef.current = hls;

    hls.loadSource(hlsUrl);
    hls.attachMedia(video);

    hls.on(Hls.Events.MANIFEST_PARSED, () => {
      hlsLevelsRef.current = hls.levels.map((l) => ({ height: l.height }));
      video.play().catch(() => {});
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
              hls.startLoad();
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

  return { transcodeStartTime, setTranscodeStartTime };
}
