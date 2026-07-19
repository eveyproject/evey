// Casting hook covering the three remote-playback modes Evey supports:
// native Chromecast (JS Cast SDK), server-side "cast" (backend-driven
// Chromecast for browsers without the SDK), and DLNA renderer fallback.
import { useState, useCallback, useEffect, useRef } from "react";
import { eveyFetch } from "@evey/api";
import { toast } from "@evey/store";
import i18n from "@/i18n";

const CAST_APP_ID = "CC1AD845";
const TAG = "[Cast]";

const DEV = import.meta.env.DEV;
const log: typeof console.log = DEV ? console.log.bind(console) : () => {};
const warn: typeof console.warn = DEV ? console.warn.bind(console) : () => {};
const error: typeof console.error = DEV ? console.error.bind(console) : () => {};

// loadChromecastMedia builds a MediaInfo + LoadRequest and hands it to an
// existing Cast session. Shared by the initial startCast and by castReload
// (the seek-by-reload path) so the HLS/mp4 content-type handling stays in
// one place. Exported for tests.
//
// startAtSec seeks the receiver to that position at load - the DIRECT-PLAY
// resume path: the receiver owns the file's whole timeline and nobody else
// will ever seek it, so omitting it restarted every direct-play cast at 0.
// A transcode cast passes 0: its start offset is baked into the stream
// server-side (startSec=) and the receiver's timeline begins there already.
export async function loadChromecastMedia(session: cast.framework.CastSession, absoluteUrl: string, title?: string, startAtSec = 0): Promise<void> {
  const chrome = window.chrome;
  const isHls = absoluteUrl.includes("m3u8");
  const contentType = isHls ? "application/x-mpegurl" : "video/mp4";
  const mediaInfo = new chrome.cast.media.MediaInfo(absoluteUrl, contentType);
  mediaInfo.streamType = chrome.cast.media.StreamType.BUFFERED;
  // Evey's HLS is fMP4/CMAF (.m4s + #EXT-X-MAP init) by default (UseFMP4),
  // muxed video+audio. The default Cast receiver needs BOTH format hints:
  // hlsVideoSegmentFormat for the video (it otherwise assumes MPEG2-TS and
  // can't demux the fMP4 - the receiver loads the manifest but buffers
  // forever with no picture), and hlsSegmentFormat for the audio. Omitting
  // the video one is why cast stalled on "loading" despite a valid stream.
  if (isHls) {
    mediaInfo.hlsVideoSegmentFormat = chrome.cast.media.HlsVideoSegmentFormat.FMP4;
    mediaInfo.hlsSegmentFormat = chrome.cast.media.HlsSegmentFormat.FMP4;
  }
  mediaInfo.metadata = new chrome.cast.media.GenericMediaMetadata();
  mediaInfo.metadata.title = title ?? "";
  const loadRequest = new chrome.cast.media.LoadRequest(mediaInfo);
  // Always autoplay: casting while the LOCAL player is paused must still
  // start playback on the TV - the handoff pauses the local element anyway.
  loadRequest.autoplay = true;
  if (startAtSec > 0) loadRequest.currentTime = startAtSec;
  await session.loadMedia(loadRequest);
}

interface DLNARenderer {
  id: string;
  name: string;
  location: string;
  controlUrl: string;
}

interface CastDevice {
  uuid: string;
  name: string;
  addr: string;
  port: number;
}

interface CastState {
  available: boolean;
  connected: boolean;
  deviceName: string | null;
  mode: "chromecast" | "cast" | "dlna" | null;
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

// Only iOS is a hard no for the Cast sender SDK (WebKit-only, no Cast
// surface). Android Chrome supports it fully - skipping all mobile here is
// what used to leave Android users with no cast button. Every other
// non-Chromium browser (desktop or Android Firefox) self-reports through
// __onGCastApiAvailable(false), so no UA gate is needed for them.
const isIOSBrowser = /iPhone|iPad|iPod/i.test(navigator.userAgent);

// The server-side Cast/DLNA fallback (/api/v1/cast/devices, /api/v1/dlna/
// renderers) discovers devices on the SERVER's LAN via mDNS/SSDP. That only
// overlaps the user's own network when Evey is reached DIRECTLY by a bare LAN
// IP (e.g. http://192.168.1.50:9091) - then the server is on the same wire as
// the user's Chromecast. Behind a domain, the server sits on a Docker bridge /
// remote host whose LAN is not the user's, so the fallback can only ever return
// [] and the "no devices on the server's network" toast is actively misleading
// (a user on Android Chrome whose Cast SDK didn't open a session lands here and
// is told, wrongly, that the SERVER has no devices). Gate the whole fallback on
// a bare-IP/loopback host. `.local` mDNS names are treated as domains: the ask
// was "just an IP", and a truly LAN-only deployment is always reachable by IP.
export function isDirectLanHost(hostname: string): boolean {
  const h = hostname.toLowerCase();
  if (h === "localhost") return true;
  if (h.includes(":")) return true; // IPv6 literal (loopback / link-local / ULA)
  return /^\d{1,3}(\.\d{1,3}){3}$/.test(h); // bare IPv4
}

function ensureCastSdk() {
  if (sdkLoaded) return;
  sdkLoaded = true;

  if (isIOSBrowser) {
    log(TAG, "iOS browser detected - skipping Cast SDK (unsupported)");
    return;
  }
  log(TAG, "Injecting Cast sender SDK script...");

  window.__onGCastApiAvailable = (isAvailable: boolean) => {
    log(TAG, "__onGCastApiAvailable:", isAvailable);
    if (!isAvailable) {
      warn(TAG, "Cast SDK not available (requires Chrome/Chromium)");
      return;
    }
    const cast = window.cast;
    const chrome = window.chrome;
    if (!cast?.framework || !chrome?.cast) {
      error(TAG, "Cast framework missing despite isAvailable=true");
      return;
    }

    try {
      cast.framework.CastContext.getInstance().setOptions({
        receiverApplicationId: CAST_APP_ID,
        autoJoinPolicy: chrome.cast.AutoJoinPolicy.ORIGIN_SCOPED,
      });
      log(TAG, "CastContext initialized - appId:", CAST_APP_ID);
    } catch (err) {
      error(TAG, "Failed to initialize CastContext:", err);
      return;
    }

    sdkReady = true;
    readyCallbacks.forEach((cb) => cb());
    readyCallbacks.length = 0;
  };

  const script = document.createElement("script");
  script.src = "https://www.gstatic.com/cv/js/sender/v1/cast_sender.js?loadCastFramework=1";
  script.async = true;
  script.onerror = (e) => error(TAG, "cast_sender.js FAILED to load:", e);
  document.head.appendChild(script);
}

export function useCasting() {
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
  // Whether the Google Cast sender SDK actually initialized. False on
  // Firefox/Safari/iOS (no SDK) - the UI uses this to hide the cast
  // button for users who have no working native-cast path. Chromium
  // desktop AND Android Chrome both initialize it.
  const [castSdkReady, setCastSdkReady] = useState(sdkReady);

  useEffect(() => {
    ensureCastSdk();

    // Keep references to all handlers for cleanup
    let ctxRef: any = null;
    let rcRef: any = null;
    let castFw: any = null;
    const ctxHandlers: Array<[string, any]> = [];
    const rcHandlers: Array<[string, any]> = [];

    const onReady = () => {
      chromecastReady.current = true;
      setCastSdkReady(true);
      const ctx = window.cast?.framework?.CastContext?.getInstance();
      if (!ctx) return;
      ctxRef = ctx;
      castFw = window.cast.framework;

      const onCastStateChanged = (e: any) => {
        log(TAG, "CAST_STATE_CHANGED:", e.castState);
        if (e.castState === "NOT_CONNECTED" || e.castState === "NO_DEVICES_AVAILABLE") {
          setState((prev) => {
            if (prev.mode === "chromecast" && prev.connected) {
              return { ...prev, connected: false, deviceName: null, mode: null };
            }
            return prev;
          });
          setPlayback({ playerState: null, currentTime: 0, duration: 0, isPaused: true, isMediaLoaded: false, volume: 1, isMuted: false });
        }
      };
      ctx.addEventListener(castFw.CastContextEventType.CAST_STATE_CHANGED, onCastStateChanged);
      ctxHandlers.push([castFw.CastContextEventType.CAST_STATE_CHANGED, onCastStateChanged]);

      const onSessionStateChanged = (e: any) => {
        log(TAG, "SESSION_STATE_CHANGED:", e.sessionState);
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
      };
      ctx.addEventListener(castFw.CastContextEventType.SESSION_STATE_CHANGED, onSessionStateChanged);
      ctxHandlers.push([castFw.CastContextEventType.SESSION_STATE_CHANGED, onSessionStateChanged]);

      const rp = new castFw.RemotePlayer();
      const rc = new castFw.RemotePlayerController(rp);
      remotePlayerRef.current = rp;
      controllerRef.current = rc;
      rcRef = rc;
      const ev = castFw.RemotePlayerEventType;

      const onPlayerState = () => {
        log(TAG, "PLAYER_STATE:", rp.playerState);
        setPlayback((prev) => ({ ...prev, playerState: rp.playerState, isPaused: rp.isPaused }));
      };
      rc.addEventListener(ev.PLAYER_STATE_CHANGED, onPlayerState);
      rcHandlers.push([ev.PLAYER_STATE_CHANGED, onPlayerState]);

      let lastLoggedTime = 0;
      const onCurrentTime = () => {
        setPlayback((prev) => ({ ...prev, currentTime: rp.currentTime }));
        if (Math.floor(rp.currentTime) % 30 === 0 && Math.floor(rp.currentTime) !== lastLoggedTime) {
          lastLoggedTime = Math.floor(rp.currentTime);
          log(TAG, "CURRENT_TIME:", rp.currentTime);
        }
      };
      rc.addEventListener(ev.CURRENT_TIME_CHANGED, onCurrentTime);
      rcHandlers.push([ev.CURRENT_TIME_CHANGED, onCurrentTime]);

      const onDuration = () => setPlayback((prev) => ({ ...prev, duration: rp.duration }));
      rc.addEventListener(ev.DURATION_CHANGED, onDuration);
      rcHandlers.push([ev.DURATION_CHANGED, onDuration]);

      const onMediaLoaded = () => {
        log(TAG, "IS_MEDIA_LOADED:", rp.isMediaLoaded, "duration:", rp.duration);
        setPlayback((prev) => ({ ...prev, isMediaLoaded: rp.isMediaLoaded, duration: rp.duration }));
      };
      rc.addEventListener(ev.IS_MEDIA_LOADED_CHANGED, onMediaLoaded);
      rcHandlers.push([ev.IS_MEDIA_LOADED_CHANGED, onMediaLoaded]);

      const onPaused = () => setPlayback((prev) => ({ ...prev, isPaused: rp.isPaused }));
      rc.addEventListener(ev.IS_PAUSED_CHANGED, onPaused);
      rcHandlers.push([ev.IS_PAUSED_CHANGED, onPaused]);

      const onVolume = () => setPlayback((prev) => ({ ...prev, volume: rp.volumeLevel }));
      rc.addEventListener(ev.VOLUME_LEVEL_CHANGED, onVolume);
      rcHandlers.push([ev.VOLUME_LEVEL_CHANGED, onVolume]);

      const onMuted = () => setPlayback((prev) => ({ ...prev, isMuted: rp.isMuted }));
      rc.addEventListener(ev.IS_MUTED_CHANGED, onMuted);
      rcHandlers.push([ev.IS_MUTED_CHANGED, onMuted]);
    };

    if (sdkReady) onReady();
    else readyCallbacks.push(onReady);

    return () => {
      // Remove from pending callbacks if not yet fired
      const idx = readyCallbacks.indexOf(onReady);
      if (idx >= 0) readyCallbacks.splice(idx, 1);
      // Remove CastContext listeners
      if (ctxRef) {
        for (const [type, handler] of ctxHandlers) {
          ctxRef.removeEventListener(type, handler);
        }
      }
      // Remove RemotePlayerController listeners
      if (rcRef) {
        for (const [type, handler] of rcHandlers) {
          rcRef.removeEventListener(type, handler);
        }
      }
      controllerRef.current = null;
      remotePlayerRef.current = null;
    };
  }, []);

  // startAtSec: initial receiver position for DIRECT PLAY (see
  // loadChromecastMedia); transcode casts pass 0 (offset baked in the URL).
  const startCast = useCallback(async (streamUrl: string, title?: string, startAtSec = 0) => {
    log(TAG, "startCast - url:", streamUrl, "title:", title, "startAtSec:", startAtSec);

    const origin = localStorage.getItem("cast_base_url") || window.location.origin;
    const absoluteUrl = streamUrl.startsWith("http")
      ? streamUrl
      : `${origin}${streamUrl}`;
    log(TAG, "Absolute URL:", absoluteUrl);

    if (chromecastReady.current) {
      try {
        const cast = window.cast;
        const ctx = cast.framework.CastContext.getInstance();

        let session = ctx.getCurrentSession();
        if (!session) {
          await ctx.requestSession();
          session = ctx.getCurrentSession();
        }
        if (!session) {
          warn(TAG, "No session - user cancelled");
          return;
        }

        const device = session.getCastDevice();
        log(TAG, "Connected to:", device?.friendlyName);

        log(TAG, "Loading media...", { url: absoluteUrl });
        try {
          await loadChromecastMedia(session, absoluteUrl, title, startAtSec);
        } catch (loadErr) {
          // A session exists (the user picked a device) but the receiver
          // couldn't load the stream. Falling through to server-side LAN
          // discovery here is wrong - it just yields a misleading "no devices"
          // toast while the film keeps playing locally. Surface the real
          // failure and stop.
          error(TAG, "Chromecast loadMedia failed:", loadErr);
          toast.error(i18n.t("player.cast.loadFailed"));
          return;
        }
        log(TAG, "Media loaded on Chromecast");

        setState((prev) => ({
          ...prev,
          connected: true,
          deviceName: device?.friendlyName ?? null,
          mode: "chromecast",
        }));
        return;
      } catch (err: unknown) {
        const e = err as { code?: string; description?: string };
        error(TAG, "Chromecast session failed:", e.code, e.description);
        // User dismissed the device picker, or a session error before load.
        // Only a genuine "no native SDK" situation should reach the server-side
        // fallback below; a cancel just aborts.
        if (e.code === "cancel") return;
      }
    }

    // Server-side Cast (Chromecast via backend) + DLNA fallback - ONLY when
    // Evey is reached directly by a LAN IP, where the server shares the user's
    // network. Behind a domain (reverse proxy) it never can, so skip the whole
    // server round-trip and give an honest client-side message instead of the
    // misleading "no devices on the server's network" toast. Applies to admins
    // too: an admin on a domain has the same non-overlapping-LAN problem.
    let castHost = window.location.hostname;
    try {
      castHost = new URL(absoluteUrl).hostname;
    } catch {
      /* absoluteUrl always absolute here; fall back to the page host */
    }
    if (!isDirectLanHost(castHost)) {
      toast.error(i18n.t("player.cast.notFound"));
      return;
    }

    try {
      const castDevices = (await eveyFetch<CastDevice[]>({
        url: "/api/v1/cast/devices",
        method: "GET",
      })) ?? [];

      if (castDevices.length > 0) {
        const device = castDevices[0];
        const isHls = absoluteUrl.includes("m3u8");
        await eveyFetch({
          url: "/api/v1/cast/play",
          method: "POST",
          data: {
            deviceUuid: device.uuid,
            mediaUrl: absoluteUrl,
            title: title ?? "",
            contentType: isHls ? "application/x-mpegurl" : "video/mp4",
          },
        });
        activeRenderer.current = device.uuid;
        setState((prev) => ({
          ...prev,
          connected: true,
          deviceName: device.name,
          mode: "cast",
        }));
        return;
      }

      // DLNA fallback
      const renderers = (await eveyFetch<DLNARenderer[]>({
        url: "/api/v1/dlna/renderers",
        method: "GET",
      })) ?? [];
      if (renderers.length === 0) {
        // Server-side discovery found nothing on the server's LAN. Tell
        // the user instead of failing silently - the most common cause
        // is the evey container being on a Docker bridge network where
        // mDNS/SSDP multicast can't reach the LAN.
        toast.error(i18n.t("player.cast.noDevices"));
        return;
      }
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
      error(TAG, "Cast/DLNA fallback failed:", err);
    }
  }, []);

  const stopCast = useCallback(async () => {
    log(TAG, "stopCast - mode:", state.mode);
    if (state.mode === "chromecast") {
      try {
        const ctx = window.cast?.framework?.CastContext?.getInstance();
        ctx?.getCurrentSession()?.endSession(true);
      } catch (err) {
        error(TAG, "Error ending session:", err);
      }
    } else if (state.mode === "cast" && activeRenderer.current) {
      try {
        await eveyFetch({
          url: "/api/v1/cast/stop",
          method: "POST",
          data: { deviceUuid: activeRenderer.current },
        });
      } catch (err) {
        error(TAG, "Cast stop failed:", err);
      }
      activeRenderer.current = null;
    } else if (state.mode === "dlna" && activeRenderer.current) {
      try {
        await eveyFetch({
          url: "/api/v1/dlna/stop",
          method: "POST",
          data: { rendererId: activeRenderer.current },
        });
      } catch (err) {
        error(TAG, "DLNA stop failed:", err);
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

  // castReload swaps the media playing on the *current* cast session to a
  // new URL. It's the cast-side equivalent of the local player's
  // setTranscodeStartTime: an HLS cast plays a progressively-generated
  // manifest, so the only way to reach a position outside that window is
  // to rebuild the URL at startSec=target and reload. Dispatches by mode -
  // the Chrome path uses the JS Cast SDK (session.loadMedia), the
  // server-side path (Firefox, no JS SDK) re-issues /api/v1/cast/play.
  // Returns false when there's no live session to reload onto.
  const castReload = useCallback(async (streamUrl: string, title?: string): Promise<boolean> => {
    const origin = localStorage.getItem("cast_base_url") || window.location.origin;
    const absoluteUrl = streamUrl.startsWith("http") ? streamUrl : `${origin}${streamUrl}`;
    if (state.mode === "chromecast") {
      const ctx = window.cast?.framework?.CastContext?.getInstance();
      const session = ctx?.getCurrentSession();
      if (!session) {
        warn(TAG, "castReload: no chromecast session");
        return false;
      }
      try {
        await loadChromecastMedia(session, absoluteUrl, title);
        return true;
      } catch (err) {
        error(TAG, "castReload chromecast failed:", err);
        return false;
      }
    }
    if (state.mode === "cast" && activeRenderer.current) {
      try {
        await eveyFetch({
          url: "/api/v1/cast/play",
          method: "POST",
          data: {
            deviceUuid: activeRenderer.current,
            mediaUrl: absoluteUrl,
            title: title ?? "",
            contentType: absoluteUrl.includes("m3u8") ? "application/x-mpegurl" : "video/mp4",
          },
        });
        return true;
      } catch (err) {
        error(TAG, "castReload server-side failed:", err);
        return false;
      }
    }
    return false;
  }, [state.mode]);

  // Returns true only when the seek was actually handed to a live cast
  // session. A false return lets the caller fall back to the local
  // player - critical when a cast session dies uncleanly (no
  // SESSION_ENDED event) and leaves `connected`/`isMediaLoaded` stuck
  // true: without the fallback every local seek is silently swallowed
  // by this dead RemotePlayer.
  const castSeek = useCallback((timeSec: number): boolean => {
    const rp = remotePlayerRef.current;
    log(TAG, "castSeek:", timeSec, "canSeek:", rp?.canSeek, "currentTime:", rp?.currentTime, "duration:", rp?.duration);
    if (rp && rp.canSeek) {
      rp.currentTime = timeSec;
      controllerRef.current?.seek();
      return true;
    }
    warn(TAG, "castSeek: cannot seek - canSeek:", rp?.canSeek);
    return false;
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
    castSdkReady,
    castPlayback: playback,
    startCast,
    stopCast,
    castPlayPause,
    castSeek,
    castReload,
    castSetVolume,
    castSetMuted,
  };
}
