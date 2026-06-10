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
  // Height (px) of the rung hls.js is actually playing right now. Surfaced so the
  // settings panel can show "Auto (1080p)" instead of a redundant bare "Auto".
  const [autoLevelHeight, setAutoLevelHeight] = useState(0);

  useEffect(() => {
    if (!transcodeMode) return;
    const video = videoRef.current;
    if (!video) return;

    if (hlsRef.current) {
      hlsRef.current.destroy();
      hlsRef.current = null;
    }

    // VOD pipeline: the server precomputes the full manifest covering
    // the entire media (0 → totalDuration) and serves segments on
    // demand. There is no "transcode start offset" — the playhead is
    // an absolute position in the source. transcodeOffset must stay
    // at 0 so positionRef = currentTime matches the absolute time;
    // a non-zero offset (legacy sliding-window assumption) silently
    // produced UI/playhead drift on resume (UI showed 9:45 but
    // playback started at segment 0).
    //
    // Positioning comes from the PLAYLIST, not the fragment's internal
    // timestamp: hls.js places each segment at its absolute EXTINF offset in
    // the precalculated VOD manifest and applies a per-segment timestampOffset.
    // Each ffmpeg session resets the fragment's fMP4 tfdt to 0 (a fresh session
    // always does, with or without -copyts — verified against jellyfin-ffmpeg7),
    // and hls.js tolerates that because the EXTINF drives the timeline. So a
    // seek-spawned rafale at segment N lands at N's manifest position regardless
    // of its tfdt.
    transcodeOffsetRef.current = 0;
    const initialSeekSec = transcodeStartTime > 0 ? transcodeStartTime : 0;

    const hlsUrl = buildTranscodeHlsUrl(
      mediaId,
      streamTokenRef.current,
      currentTrackIndex,
      selectedBurnInTrack,
      0, // server ignores startSec on the VOD path
    );

    if (!Hls.isSupported()) {
      video.src = hlsUrl;
      // Native-HLS path (Safari/iOS, where Hls.isSupported() is false). The
      // stream is transcoded starting at `offset`, so video.currentTime is
      // local to the restarted segment — positionRef must hold the ABSOLUTE
      // position (offset + local) like the Hls.js path does, otherwise every
      // seek after an out-of-buffer restart on Safari lands offset seconds off.
      //
      // currentTime only sticks once metadata is loaded on native HLS, so the
      // resume/restart target is applied in loadedmetadata (mirroring the
      // hls.js MANIFEST_PARSED path). Without this the branch hard-seeked to 0,
      // so every resume from watch progress and every audio/subtitle restart on
      // Safari jumped back to the start of the media.
      function onLoadedMetadata() {
        if (initialSeekSec > 0) video!.currentTime = initialSeekSec;
        if (!wasPausedRef.current) {
          video!.play().catch(() => {
            video!.muted = true;
            void video!.play().catch(() => { /* user gesture required */ });
          });
        }
      }
      function onTimeUpdate() { positionRef.current = transcodeOffsetRef.current + video!.currentTime; }
      function handleEnded() { onEnded?.(); }
      video.addEventListener("loadedmetadata", onLoadedMetadata);
      video.addEventListener("timeupdate", onTimeUpdate);
      video.addEventListener("ended", handleEnded);
      return () => {
        wasPausedRef.current = video.paused;
        video.removeEventListener("loadedmetadata", onLoadedMetadata);
        video.removeEventListener("timeupdate", onTimeUpdate);
        video.removeEventListener("ended", handleEnded);
        video.src = "";
      };
    }

    const hls = new Hls({
      // -1 lets hls.js compute the start position from the manifest
      // (it'll seek to whatever we set via video.currentTime in
      // MANIFEST_PARSED). Forcing 0 here defeats the resume.
      startPosition: -1,
      maxBufferLength: 30,
      maxMaxBufferLength: 60,
      // Long-distance seeks spawn a fresh rafale on the server side;
      // ffmpeg can take 10-15 s to produce segment 0 of that rafale
      // on a 4K HEVC source. The hls.js default fragLoadingTimeOut
      // (20 s) is right at the edge and can fire when the rafale
      // races startup → "fragLoadTimeOut" surfaced as a fatal error
      // on legitimate seeks. Bump the per-fragment timeout to 60 s;
      // the orchestrator's own segmentWaitTimeout (25 s) keeps a
      // genuinely stuck request from hanging indefinitely.
      fragLoadingTimeOut: 60_000,
      fragLoadingMaxRetry: 4,
      // Emit CMCD (CTA-5004) on every segment / playlist request. The
      // server reads br, bl, mtp, bs, su, sid, ... and stores them in
      // playback_qoe_events for the /admin/qoe dashboard, plus uses
      // bs=1 to wake up a throttled ffmpeg immediately. Defaults: query
      // mode (?CMCD=...), auto-generated sessionId.
      cmcd: {},
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

    // Track the rung ABR actually settled on, so "Auto" can show its resolution.
    hls.on(Hls.Events.LEVEL_SWITCHED, (_evt, data) => {
      const h = hls.levels?.[data.level]?.height;
      if (h) setAutoLevelHeight(h);
    });

    hls.on(Hls.Events.MANIFEST_PARSED, () => {
      setHlsLevels(hls.levels.map((l) => l.height));
      // NB: hls.js ABR is intentionally NOT used on this transcode path — the
      // level-sync effect in HLSPlayer PINS a single rung (top for Auto, the chosen
      // one for manual) because free ABR loads several rungs at once on a transcode
      // box and thrashes. So no startLevel / abrEwmaDefaultEstimate tuning here.
      // VOD pipeline: the manifest covers the full media duration so we
      // seek the resume target via video.currentTime, which causes
      // hls.js to request the matching segment from the server.
      // SegmentOrchestrator (or the copy rung) spawns a rafale at that segment
      // with the matching -ss; hls.js accepts it because the playlist's EXTINF
      // places the segment — the fragment's tfdt resets to 0 per session and is
      // tolerated (positioning is by the manifest, not the fragment timestamp).
      if (initialSeekSec > 0) {
        video.currentTime = initialSeekSec;
      } else if (video.currentTime !== 0) {
        video.currentTime = 0;
      }
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
              // The token refresh is async; if the effect re-ran meanwhile
              // (audio/subtitle/quality switch destroyed this instance and
              // built a new one), don't drive the dead instance — the live
              // one in hlsRef is already loading.
              if (hls !== hlsRef.current) return;
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
        setTimeout(() => { if (hls === hlsRef.current) hls.startLoad(); }, delay);
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
      // Clear any in-flight buffering spinner when the instance is torn down
      // (audio/subtitle/quality switch rebuilds it); the fresh instance's
      // waiting/playing events re-drive the state. Avoids a stale spinner
      // surviving the transition.
      setIsBuffering(false);
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps -- streamTokenRef/onStreamTokenRefreshRef read imperatively; setFatalError/setIsBuffering/setTranscodeStartTime are stable setState
  }, [transcodeMode, mediaId, onEnded, currentTrackIndex, transcodeStartTime, selectedBurnInTrack]);

  return { transcodeStartTime, setTranscodeStartTime, hlsLevels, autoLevelHeight };
}
