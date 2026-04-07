import { useState, useCallback, useEffect, useRef } from "react";
import { eveyFetch } from "@evey/api";

const CAST_APP_ID = "CC1AD845";
const TAG = "[Cast]";

interface DLNARenderer {
  id: string;
  name: string;
  location: string;
  controlUrl: string;
}

interface CastState {
  available: boolean;
  connected: boolean;
  deviceName: string | null;
  mode: "chromecast" | "dlna" | null;
}

interface CastPlayback {
  playerState: string | null;
  currentTime: number;
  duration: number;
  isPaused: boolean;
  isMediaLoaded: boolean;
  volume: number;
  isMuted: boolean;
}

let sdkLoaded = false;
let sdkReady = false;
const readyCallbacks: Array<() => void> = [];

const isMobileBrowser = /Android|iPhone|iPad|iPod|Mobile/i.test(navigator.userAgent);

function ensureCastSdk() {
  if (sdkLoaded) return;
  sdkLoaded = true;

  if (isMobileBrowser) {
    console.log(TAG, "Mobile browser detected — skipping Cast SDK (unsupported)");
    return;
  }
  console.log(TAG, "Injecting Cast sender SDK script...");

  (window as any).__onGCastApiAvailable = (isAvailable: boolean) => {
    console.log(TAG, "__onGCastApiAvailable:", isAvailable);
    if (!isAvailable) {
      console.warn(TAG, "Cast SDK not available (requires Chrome/Chromium)");
      return;
    }
    const cast = (window as any).cast;
    const chrome = (window as any).chrome;
    if (!cast?.framework || !chrome?.cast) {
      console.error(TAG, "Cast framework missing despite isAvailable=true");
      return;
    }

    try {
      cast.framework.CastContext.getInstance().setOptions({
        receiverApplicationId: CAST_APP_ID,
        autoJoinPolicy: chrome.cast.AutoJoinPolicy.ORIGIN_SCOPED,
      });
      console.log(TAG, "CastContext initialized — appId:", CAST_APP_ID);
    } catch (err) {
      console.error(TAG, "Failed to initialize CastContext:", err);
      return;
    }

    sdkReady = true;
    readyCallbacks.forEach((cb) => cb());
    readyCallbacks.length = 0;
  };

  const script = document.createElement("script");
  script.src = "https://www.gstatic.com/cv/js/sender/v1/cast_sender.js?loadCastFramework=1";
  script.async = true;
  script.onerror = (e) => console.error(TAG, "cast_sender.js FAILED to load:", e);
  document.head.appendChild(script);
}

export function useDLNA() {
  const [state, setState] = useState<CastState>({
    available: true,
    connected: false,
    deviceName: null,
    mode: null,
  });
  const [playback, setPlayback] = useState<CastPlayback>({
    playerState: null,
    currentTime: 0,
    duration: 0,
    isPaused: true,
    isMediaLoaded: false,
    volume: 1,
    isMuted: false,
  });
  const activeRenderer = useRef<string | null>(null);
  const chromecastReady = useRef(false);
  const controllerRef = useRef<any>(null);
  const remotePlayerRef = useRef<any>(null);

  useEffect(() => {
    ensureCastSdk();

    const onReady = () => {
      chromecastReady.current = true;
      const ctx = (window as any).cast?.framework?.CastContext?.getInstance();
      if (!ctx) return;

      const castFw = (window as any).cast.framework;

      ctx.addEventListener(castFw.CastContextEventType.CAST_STATE_CHANGED, (e: any) => {
        console.log(TAG, "CAST_STATE_CHANGED:", e.castState);
        if (e.castState === "NOT_CONNECTED" || e.castState === "NO_DEVICES_AVAILABLE") {
          setState((prev) => {
            if (prev.mode === "chromecast" && prev.connected) {
              return { ...prev, connected: false, deviceName: null, mode: null };
            }
            return prev;
          });
          setPlayback({ playerState: null, currentTime: 0, duration: 0, isPaused: true, isMediaLoaded: false, volume: 1, isMuted: false });
        }
      });

      ctx.addEventListener(castFw.CastContextEventType.SESSION_STATE_CHANGED, (e: any) => {
        console.log(TAG, "SESSION_STATE_CHANGED:", e.sessionState);
        const session = ctx.getCurrentSession();
        if (e.sessionState === "SESSION_STARTED" || e.sessionState === "SESSION_RESUMED") {
          setState((prev) => ({
            ...prev,
            connected: true,
            deviceName: session?.getCastDevice()?.friendlyName ?? null,
            mode: "chromecast",
          }));
        } else if (e.sessionState === "SESSION_ENDED") {
          setState((prev) => ({ ...prev, connected: false, deviceName: null, mode: null }));
          setPlayback({ playerState: null, currentTime: 0, duration: 0, isPaused: true, isMediaLoaded: false, volume: 1, isMuted: false });
        }
      });

      // Remote player for playback state + controls
      const rp = new castFw.RemotePlayer();
      const rc = new castFw.RemotePlayerController(rp);
      remotePlayerRef.current = rp;
      controllerRef.current = rc;
      const ev = castFw.RemotePlayerEventType;

      rc.addEventListener(ev.PLAYER_STATE_CHANGED, () => {
        console.log(TAG, "PLAYER_STATE:", rp.playerState);
        setPlayback((prev) => ({
          ...prev,
          playerState: rp.playerState,
          isPaused: rp.isPaused,
        }));
      });
      let lastLoggedTime = 0;
      rc.addEventListener(ev.CURRENT_TIME_CHANGED, () => {
        setPlayback((prev) => ({ ...prev, currentTime: rp.currentTime }));
        if (Math.floor(rp.currentTime) % 30 === 0 && Math.floor(rp.currentTime) !== lastLoggedTime) {
          lastLoggedTime = Math.floor(rp.currentTime);
          console.log(TAG, "CURRENT_TIME:", rp.currentTime);
        }
      });
      rc.addEventListener(ev.DURATION_CHANGED, () => {
        setPlayback((prev) => ({ ...prev, duration: rp.duration }));
      });
      rc.addEventListener(ev.IS_MEDIA_LOADED_CHANGED, () => {
        console.log(TAG, "IS_MEDIA_LOADED:", rp.isMediaLoaded, "duration:", rp.duration);
        setPlayback((prev) => ({
          ...prev,
          isMediaLoaded: rp.isMediaLoaded,
          duration: rp.duration,
        }));
      });
      rc.addEventListener(ev.IS_PAUSED_CHANGED, () => {
        setPlayback((prev) => ({ ...prev, isPaused: rp.isPaused }));
      });
      rc.addEventListener(ev.VOLUME_LEVEL_CHANGED, () => {
        setPlayback((prev) => ({ ...prev, volume: rp.volumeLevel }));
      });
      rc.addEventListener(ev.IS_MUTED_CHANGED, () => {
        setPlayback((prev) => ({ ...prev, isMuted: rp.isMuted }));
      });
    };

    if (sdkReady) onReady();
    else readyCallbacks.push(onReady);
  }, []);

  const startCast = useCallback(async (streamUrl: string, title?: string) => {
    console.log(TAG, "startCast — url:", streamUrl, "title:", title);

    const origin = localStorage.getItem("cast_base_url") || window.location.origin;
    const absoluteUrl = streamUrl.startsWith("http")
      ? streamUrl
      : `${origin}${streamUrl}`;
    console.log(TAG, "Absolute URL:", absoluteUrl);

    if (chromecastReady.current) {
      try {
        const cast = (window as any).cast;
        const chrome = (window as any).chrome;
        const ctx = cast.framework.CastContext.getInstance();

        let session = ctx.getCurrentSession();
        if (!session) {
          await ctx.requestSession();
          session = ctx.getCurrentSession();
        }
        if (!session) {
          console.warn(TAG, "No session — user cancelled");
          return;
        }

        const device = session.getCastDevice();
        console.log(TAG, "Connected to:", device?.friendlyName);

        const isHls = absoluteUrl.includes("m3u8");
        const contentType = isHls ? "application/x-mpegurl" : "video/mp4";
        const mediaInfo = new chrome.cast.media.MediaInfo(absoluteUrl, contentType);
        mediaInfo.streamType = chrome.cast.media.StreamType.BUFFERED;
        if (isHls) mediaInfo.hlsSegmentFormat = chrome.cast.media.HlsSegmentFormat.TS;
        mediaInfo.metadata = new chrome.cast.media.GenericMediaMetadata();
        mediaInfo.metadata.title = title ?? "";

        const loadRequest = new chrome.cast.media.LoadRequest(mediaInfo);
        loadRequest.autoplay = true;
        console.log(TAG, "Loading media...", { url: absoluteUrl, contentType });

        await session.loadMedia(loadRequest);
        console.log(TAG, "Media loaded on Chromecast");

        setState((prev) => ({
          ...prev,
          connected: true,
          deviceName: device?.friendlyName ?? null,
          mode: "chromecast",
        }));
        return;
      } catch (err: any) {
        console.error(TAG, "Chromecast failed:", err?.code, err?.description);
        if (err?.code === "cancel") return;
      }
    }

    // DLNA fallback
    try {
      const renderers = (await eveyFetch<DLNARenderer[]>({
        url: "/api/v1/dlna/renderers",
        method: "GET",
      })) ?? [];
      if (renderers.length === 0) return;
      const renderer = renderers[0];
      await eveyFetch({
        url: "/api/v1/dlna/play",
        method: "POST",
        data: { rendererId: renderer.id, mediaUrl: absoluteUrl, title: title ?? "" },
      });
      activeRenderer.current = renderer.id;
      setState((prev) => ({
        ...prev,
        connected: true,
        deviceName: renderer.name,
        mode: "dlna",
      }));
    } catch (err) {
      console.error(TAG, "DLNA fallback failed:", err);
    }
  }, []);

  const stopCast = useCallback(async () => {
    console.log(TAG, "stopCast — mode:", state.mode);
    if (state.mode === "chromecast") {
      try {
        const ctx = (window as any).cast?.framework?.CastContext?.getInstance();
        ctx?.getCurrentSession()?.endSession(true);
      } catch (err) {
        console.error(TAG, "Error ending session:", err);
      }
    } else if (state.mode === "dlna" && activeRenderer.current) {
      try {
        await eveyFetch({
          url: "/api/v1/dlna/stop",
          method: "POST",
          data: { rendererId: activeRenderer.current },
        });
      } catch (err) {
        console.error(TAG, "DLNA stop failed:", err);
      }
      activeRenderer.current = null;
    }
    setState((prev) => ({ ...prev, connected: false, deviceName: null, mode: null }));
    setPlayback({ playerState: null, currentTime: 0, duration: 0, isPaused: true, isMediaLoaded: false, volume: 1, isMuted: false });
  }, [state.mode]);

  // Remote controls for Chromecast
  const castPlayPause = useCallback(() => {
    controllerRef.current?.playOrPause();
  }, []);

  const castSeek = useCallback((timeSec: number) => {
    const rp = remotePlayerRef.current;
    console.log(TAG, "castSeek:", timeSec, "canSeek:", rp?.canSeek, "currentTime:", rp?.currentTime, "duration:", rp?.duration);
    if (rp && rp.canSeek) {
      rp.currentTime = timeSec;
      controllerRef.current?.seek();
    } else {
      console.warn(TAG, "castSeek: cannot seek — canSeek:", rp?.canSeek);
    }
  }, []);

  const castSetVolume = useCallback((level: number) => {
    const rp = remotePlayerRef.current;
    if (rp && rp.canControlVolume) {
      rp.volumeLevel = level;
      controllerRef.current?.setVolumeLevel();
    }
  }, []);

  const castSetMuted = useCallback((muted: boolean) => {
    const rp = remotePlayerRef.current;
    if (rp) {
      rp.isMuted = muted;
      controllerRef.current?.muteOrUnmute();
    }
  }, []);

  return {
    ...state,
    castPlayback: playback,
    startCast,
    stopCast,
    castPlayPause,
    castSeek,
    castSetVolume,
    castSetMuted,
  };
}
