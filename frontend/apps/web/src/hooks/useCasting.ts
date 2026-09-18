// Casting hook covering the three remote-playback modes Evey supports:
// native Chromecast (JS Cast SDK), server-side "cast" (backend-driven
// Chromecast for browsers without the SDK), and DLNA renderer fallback.
import { useState, useCallback, useEffect, useRef } from "react";
import { eveyFetch } from "@evey/api";
import type { ObservedReceiverCaps } from "@evey/media-caps/cast";
import { toast } from "@evey/store";
import i18n from "@/i18n";

// Custom Evey Web Receiver (Cast Console D36A449B -> https://cast.eveyproject.org):
// it probes the device's REAL capabilities (canDisplayType) and decodes
// AC-3/E-AC-3 on the TV via ac3go wasm - the Default Media Receiver
// ("CC1AD845") could do neither and failed silently.
// Warning: while the app console is Unpublished, only registered test
// devices can launch it - publish BEFORE any public deployment.
const CAST_APP_ID = "D36A449B";
const TAG = "[Cast]";

const DEV = import.meta.env.DEV;
const log: typeof console.log = DEV ? console.log.bind(console) : () => {};
const warn: typeof console.warn = DEV ? console.warn.bind(console) : () => {};
const error: typeof console.error = DEV ? console.error.bind(console) : () => {};

// loadChromecastMedia builds a MediaInfo + LoadRequest and hands it to an
// existing Cast session. Exported for tests.
//
// startAtSec seeks the receiver to that position at load - the resume path
// for BOTH modes: direct play hands the receiver the complete file, transcode
// the complete precomputed VOD manifest, so either timeline is absolute and
// the load request's currentTime is the only thing that ever resumes it
// (baking startSec into the URL never worked - no server handler parses it).
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

// CastTarget is what the sender learned about the device it is about to hand
// a stream to, so the caller can negotiate that stream with the server for
// THAT device (the Web Sender SDK exposes no model name: the receiver's own
// observed caps are the only capability signal on the SDK path).
//   sdk: our receiver app answered the caps channel (caps) or did not (null:
//        an older receiver build, the Default Media Receiver, a timeout);
//   server: the backend found the device; model is its mDNS record when the
//        discovery had one (empty for a subnet-probed device);
//   dlna: a DLNA renderer, which plays raw files and is not a Cast receiver.
export type CastTarget =
  | { kind: "sdk"; caps: ObservedReceiverCaps | null; userAgent: string | null }
  | { kind: "server"; model: string | null }
  | { kind: "dlna" };

// CastUrlResolver builds the media for the target once it is known - AFTER
// the session (SDK) or the discovery (server) told us what it is, never
// before the tap from the browser's own negotiation.
export type CastUrlResolver = (target: CastTarget) => Promise<{ url: string; castResumeSec: number }>;

// CAPS_NAMESPACE and the request/answer shape are the receiver's contract
// (apps/web cast-receiver/receiverLogic.ts: CAPS_NAMESPACE, capsReport).
export const CAPS_NAMESPACE = "urn:x-cast:org.eveyproject.caps";
export const CAPS_TIMEOUT_MS = 1500;

// askReceiverCaps asks the connected receiver what it observed about itself
// and waits at most timeoutMs. null = no usable answer (a receiver that does
// not speak the channel, a malformed reply, the timeout): the caller falls
// back to the floor profile. The listener is always removed. Exported for
// tests.
export function askReceiverCaps(
  session: Pick<cast.framework.CastSession, "sendMessage" | "addMessageListener" | "removeMessageListener">,
  timeoutMs = CAPS_TIMEOUT_MS,
): Promise<{ caps: ObservedReceiverCaps; userAgent: string | null } | null> {
  return new Promise((resolve) => {
    let done = false;
    const finish = (value: { caps: ObservedReceiverCaps; userAgent: string | null } | null) => {
      if (done) return;
      done = true;
      clearTimeout(timer);
      try { session.removeMessageListener(CAPS_NAMESPACE, onMessage); } catch { /* session gone */ }
      resolve(value);
    };
    const onMessage = (_ns: string, message: string) => {
      try {
        const data = JSON.parse(message) as { type?: string; caps?: Partial<ObservedReceiverCaps>; userAgent?: string };
        const c = data?.type === "caps" ? data.caps : undefined;
        if (!c || !Array.isArray(c.videoCodecs) || typeof c.maxHeight !== "number") {
          return; // not the answer we asked for: keep waiting
        }
        const videoCodecs: unknown[] = c.videoCodecs;
        const maxHeight: number = c.maxHeight;
        finish({
          caps: {
            videoCodecs: videoCodecs.filter((v): v is string => typeof v === "string"),
            maxHeight,
            hdr: c.hdr === true,
            nativeDolby: c.nativeDolby === true,
            maxAudioChannels: typeof c.maxAudioChannels === "number" && c.maxAudioChannels > 0 ? c.maxAudioChannels : 2,
            maxH264Level: typeof c.maxH264Level === "number" ? c.maxH264Level : 40,
            h264MaxHeight: typeof c.h264MaxHeight === "number" ? c.h264MaxHeight : undefined,
          },
          userAgent: typeof data.userAgent === "string" ? data.userAgent : null,
        });
      } catch { /* malformed: keep waiting for a well-formed one */ }
    };
    const timer = setTimeout(() => finish(null), timeoutMs);
    try {
      session.addMessageListener(CAPS_NAMESPACE, onMessage);
      session.sendMessage(CAPS_NAMESPACE, { type: "caps" }).catch(() => finish(null));
    } catch {
      finish(null);
    }
  });
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
  // The mDNS model record when the discovery had one; empty for a device the
  // subnet probe found (TCP 8009 answers, no TXT record).
  model?: string;
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

// iOS is a hard no for the Cast sender SDK (WebKit-only, no Cast surface), so
// don't even inject the script there. Everywhere else the script self-reports.
//
// ⚠️ Chrome for ANDROID DOES initialize it - a comment here used to claim the
// opposite ("chrome.cast never materializes"), and the whole cast routing was
// built on that belief. Production telemetry (2026-07-22) says otherwise: every
// tap from an Android phone reported sdk=true. Worse, the session it then opens
// FAILS in mobile mode and only completes with "Desktop site" on - so a phone
// silently took the SDK path and hit a dead end, while the Remote Playback
// fling that works there was never reached. Route on the platform, not on this
// flag alone (see preferFling in HLSPlayer).
//
// Other browsers report __onGCastApiAvailable(false).
const isIOSBrowser = /iPhone|iPad|iPod/i.test(navigator.userAgent);

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

// Reporter shape borrowed from the player beacon (createPlayerEventReporter).
// The SDK cast path reported NOTHING until now: when a user said "casting is
// broken", the server logs held zero trace of the attempt, and diagnosing it
// meant guessing. The fling path has had this since 2026-07-21; this closes
// the other half.
type CastReporter = (e: { type: string; details?: string; fatal?: boolean }) => void;

// "no-devices" is its own outcome (not a "session-failed"): the server-side
// discovery answered and found nothing reachable. The caller decides what to
// tell the user - startCast itself never toasts for it, so a caller with a
// second path to try (the mobile fling) doesn't stack two errors on screen.
export type CastAttempt = "connected" | "dismissed" | "session-failed" | "load-failed" | "no-devices";

export function useCasting(report?: CastReporter) {
  const reportRef = useRef<CastReporter | undefined>(report);
  reportRef.current = report;
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
  // The outcome the caller needs to decide what to do next. "session-failed"
  // is the interesting one: the picker listed the device and the session did
  // not come up, which is recoverable by another cast path (the Remote
  // Playback fling). "load-failed" is not - the receiver took the session and
  // refused the media, and handing it the same URL another way changes nothing.
  // opts.skipSdk: Chrome Android reports the Cast SDK as ready but cannot
  // complete a session with it (measured in prod 2026-07-22: sdk=true then
  // session error, while the same phone in desktop-site mode connects fine).
  // The mobile tap path passes skipSdk to go straight to the server-side cast.
  //
  // resolveUrl is called once the TARGET is known (the SDK session is up and
  // has answered - or not - the caps channel; the backend named the device),
  // so the stream is negotiated with the server for the receiver, never
  // handed over from the browser's own negotiation. On the SDK path this
  // runs after requestSession, which is the only step that needs the tap's
  // user activation; loadMedia needs none, so the awaits in between are safe.
  const startCast = useCallback(async (resolveUrl: CastUrlResolver, title?: string, opts?: { skipSdk?: boolean }): Promise<CastAttempt> => {
    log(TAG, "startCast - title:", title);
    // Same diagnostic payload as the fling path: whether the tap's activation
    // was still alive tells a browser refusal apart from a user cancelling.
    const activation = typeof navigator !== "undefined" && "userActivation" in navigator
      ? (navigator as Navigator & { userActivation?: { isActive: boolean } }).userActivation?.isActive
      : undefined;
    reportRef.current?.({
      type: "cast_prompt",
      details: `sdk=${chromecastReady.current} activation=${activation ?? "n/a"}`,
    });

    const origin = localStorage.getItem("cast_base_url") || window.location.origin;
    const absolute = (streamUrl: string) => (streamUrl.startsWith("http") ? streamUrl : `${origin}${streamUrl}`);

    if (chromecastReady.current && !opts?.skipSdk) {
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
          reportRef.current?.({ type: "cast_prompt_result", details: "dismissed" });
          return "dismissed";
        }

        const device = session.getCastDevice();
        log(TAG, "Connected to:", device?.friendlyName);

        // What the receiver observed about itself, or nothing (an older
        // receiver build, the Default Media Receiver, a timeout) - the
        // resolver then negotiates on the floor profile. The user agent is
        // telemetry: it corrects the capability tables on evidence, it never
        // drives the negotiation.
        const answer = await askReceiverCaps(session);
        reportRef.current?.({
          type: "cast_receiver_caps",
          details: answer
            ? `answered=1 codecs=${answer.caps.videoCodecs.join("/")} maxHeight=${answer.caps.maxHeight} h264=${answer.caps.h264MaxHeight ?? "level" + answer.caps.maxH264Level} hdr=${answer.caps.hdr ? 1 : 0} ch=${answer.caps.maxAudioChannels} ua=${answer.userAgent ?? "-"}`
            : "answered=0",
        });
        const { url, castResumeSec: startAtSec } = await resolveUrl({ kind: "sdk", caps: answer?.caps ?? null, userAgent: answer?.userAgent ?? null });
        const absoluteUrl = absolute(url);

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
          const le = loadErr as { code?: string; description?: string };
          reportRef.current?.({
            type: "cast_load_failed",
            details: `${le?.code ?? "unknown"}${le?.description ? `: ${le.description}` : ""}`,
            fatal: true,
          });
          toast.error(i18n.t("player.cast.loadFailed"));
          return "load-failed";
        }
        log(TAG, "Media loaded on Chromecast");
        reportRef.current?.({ type: "cast_prompt_result", details: "connected" });

        setState((prev) => ({
          ...prev,
          connected: true,
          deviceName: device?.friendlyName ?? null,
          mode: "chromecast",
        }));
        return "connected";
      } catch (err: unknown) {
        const e = err as { code?: string; description?: string };
        error(TAG, "Chromecast session failed:", e.code, e.description);
        // The SDK does not always throw a chrome.cast.Error: an object with no
        // `code` produced a useless "error:unknown" in production. Fall back to
        // the exception's name and message so the next failure is readable.
        const err2 = err as { code?: string; description?: string; name?: string; message?: string };
        const label = err2.code ?? err2.name ?? err2.message ?? "unknown";
        reportRef.current?.({
          type: "cast_prompt_result",
          details: `error:${label}${err2.description ? `: ${err2.description}` : ""}`,
          fatal: e.code !== "cancel",
        });
        // A cancel just aborts - the user closed the picker.
        if (e.code === "cancel") return "dismissed";
        // Any OTHER session error means the picker DID list the device and the
        // connection to it failed (receiver_unavailable, session_error,
        // timeout...). Falling through to the server-side LAN discovery below
        // told the user "no Chromecast found on your Wi-Fi" about a device they
        // had just selected - which sent them hunting a network problem that
        // did not exist. Hand the outcome back instead: the caller has a second
        // cast path to try, and only it knows whether that path exists here.
        return "session-failed";
      }
    }

    // Server-side Cast (Chromecast via backend) + DLNA fallback. This used to
    // be gated on reaching Evey by a LAN IP ("behind a domain the server never
    // shares the visitor's LAN") - which wrote off the most common self-hosted
    // deployment: server and visitor on the SAME home LAN, accessed through
    // the public domain. The server now probes the CLIENT's subnet during
    // discovery (see the chromecast service), so it answers the question
    // authoritatively: an empty list means genuinely nothing reachable, not
    // "wrong kind of hostname".
    try {
      const castDevices = (await eveyFetch<CastDevice[]>({
        url: "/api/v1/cast/devices",
        method: "GET",
      })) ?? [];

      if (castDevices.length > 0) {
        const device = castDevices[0];
        // The backend launches our receiver app on the device (its LOAD
        // rewrite then re-negotiates the master on its real caps) and falls
        // back to the Default Media Receiver, which rewrites nothing: the
        // URL handed here is negotiated for the device's model when the
        // discovery had one, the floor otherwise.
        const { url } = await resolveUrl({ kind: "server", model: device.model || null });
        const absoluteUrl = absolute(url);
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
        return "connected";
      }

      // DLNA fallback
      const renderers = (await eveyFetch<DLNARenderer[]>({
        url: "/api/v1/dlna/renderers",
        method: "GET",
      })) ?? [];
      if (renderers.length === 0) {
        // Nothing reachable server-side either (Chromecast probe + DLNA).
        // Hand the fact back instead of toasting here - only the caller
        // knows whether it still has a path to try before saying anything.
        return "no-devices";
      }
      const renderer = renderers[0];
      // A DLNA renderer is not a Cast receiver: it plays the raw file the
      // browser negotiated, as before.
      const { url } = await resolveUrl({ kind: "dlna" });
      const absoluteUrl = absolute(url);
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
      return "connected";
    } catch (err) {
      error(TAG, "Cast/DLNA fallback failed:", err);
    }
    return "session-failed";
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
    castSetVolume,
    castSetMuted,
  };
}
