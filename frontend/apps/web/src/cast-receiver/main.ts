// Entry point for the custom Cast Web Receiver (page /cast-receiver.html,
// declared in the Google Cast Console). Roles:
//   1. renegotiate the HLS master against the device's REAL capabilities
//      (probeReceiverCaps + rewriteCastMasterUrl - no more per-model table);
//   2. decode AC-3/E-AC-3 ON the TV via ac3go wasm when the cast runtime does
//      not do it natively (video-only master + WasmAudioEngine, the same
//      engine as the web player); automatic fallback to transcoded AAC if
//      the wasm does not start;
//   3. show playback errors ON SCREEN - the Default Media Receiver fails
//      silently, which is what made cast incidents undiagnosable.
import { WasmAudioEngine } from "@/features/playback/wasmaudio/WasmAudioEngine";
import { extractAudioURI } from "@/features/playback/wasmaudio/wasmAudioShared";
import { absoluteWithToken, buildPlayerEventUrl, probeReceiverCaps, rewriteCastMasterUrl, stripWasmAudio } from "./receiverLogic";
import { createAliveHeartbeat, postPlayerEvent } from "./receiverBeacon";

// The CAF SDK is loaded by a gstatic <script> in the page; its types are
// not packaged - deliberately untyped access, scoped to this file.
/* eslint-disable @typescript-eslint/no-explicit-any */
declare const cast: any;

const context = cast.framework.CastReceiverContext.getInstance();
const playerManager = context.getPlayerManager();

const dbg = document.getElementById("dbg");
function showDebug(text: string): void {
  if (!dbg) return;
  dbg.textContent = text;
  dbg.style.display = text ? "block" : "none";
}

let engine: WasmAudioEngine | null = null;
let wasmDisabled = false;
let currentUrl = "";
let wasmRequested = false;
// Armed after a real END_OF_STREAM (idle timeout is disabled, see the
// receiver options at the bottom); any new LOAD/PLAY disarms it.
let endShutdownTimer: ReturnType<typeof setTimeout> | null = null;
function cancelEndShutdown(): void {
  if (endShutdownTimer) {
    clearTimeout(endShutdownTimer);
    endShutdownTimer = null;
  }
}
// LOAD generation: a new LOAD invalidates any engine start in progress
// (the start is async and runs in parallel with the CAF load).
let loadGen = 0;
let engineStarting = false;

// Warmup: download (HTTP cache) and COMPILE (V8 code cache, keyed by URL,
// exploited by the worker's instantiateStreaming) the decoder during the
// splash - on a TV's CPU, Go-wasm compilation is the dominant cost of
// time-to-sound. Fully best-effort: a failure here changes nothing,
// the real load will pay for it again.
for (const asset of ["/wasm/ac3Worker.js", "/wasm/wasm_exec.js"]) {
  void fetch(asset).catch(() => {});
}
try {
  void WebAssembly.compileStreaming(fetch("/wasm/ac3go.wasm")).catch(() => {});
} catch {
  /* runtime without compileStreaming: the worker's bytes fallback handles it */
}

function destroyEngine(): void {
  engine?.destroy();
  engine = null;
}

// Telemetry beacon to the media server, authenticated by the cast stream
// token riding in currentUrl. On-screen debug is invisible once the incident
// is over (2026-07-26: TV died 3-5 min into a cast, server logs blank) -
// these land as WARN lines next to the session's ffmpeg lines.
//
// sendBeacon FIRST (transport in receiverBeacon.ts): the events that matter
// most - finished, shutdown - fire while the platform is tearing the page
// down, and a fetch() loses that race. Proof: on 2026-07-27 a CLEAN stop
// (server StopStream at 19:10:37) produced neither cast_receiver_finished nor
// cast_receiver_shutdown. sendBeacon is handed to the user agent, which
// delivers it after the document dies.
function reportEvent(type: string, details: string, fatal = false): void {
  let posSec = 0;
  try {
    posSec = playerManager.getCurrentTimeSec() || 0;
  } catch { /* player torn down already */ }
  postPlayerEvent(buildPlayerEventUrl(currentUrl), { type, details, fatal, posSec }, {
    sendBeacon: typeof navigator !== "undefined" && navigator.sendBeacon
      ? (u, b) => navigator.sendBeacon(u, b)
      : undefined,
    fetchFn: (u, init) => void fetch(u, init).catch(() => {}),
  });
}

// Periodic proof of life. The whole point: a TV that dies without managing to
// say anything (platform kill, renderer crash, power cut) leaves its LAST
// heartbeat as the timestamp of its death, to the half-minute. Two incidents
// in a row went undiagnosed for exactly the lack of it. Armed on LOAD
// COMPLETE, disarmed on anything that ends playback.
const aliveHeartbeat = createAliveHeartbeat(() => {
  reportEvent("cast_receiver_alive", `mode=${wasmRequested ? "wasm" : "native"}`);
});

// tokenFromUrl reads the purpose=cast token the sender baked into the URL - the
// receiver has no other identity, this token is ALL of its authorization.
function tokenFromUrl(rawUrl: string): string {
  try {
    return new URL(rawUrl).searchParams.get("token") ?? "";
  } catch {
    return "";
  }
}

// findMediaElement fetches the <video> the CAF player renders in the shadow
// DOM of <cast-media-player>. No public API for this: if Google closes the
// shadow root off one day, returning null simply triggers the AAC fallback -
// never a silent black screen.
function findMediaElement(): HTMLVideoElement | null {
  const cmp = document.querySelector("cast-media-player");
  const inShadow = cmp?.shadowRoot?.querySelector("video");
  if (inShadow) return inShadow as HTMLVideoElement;
  return document.querySelector("video");
}

// Fallback: reload the SAME content without wasmAudio - the server re-serves
// the muxed AAC master. wasmDisabled is a latch: we don't retry in a loop.
function fallbackToMuxedAudio(reason: string): void {
  if (!wasmRequested) return;
  wasmRequested = false;
  wasmDisabled = true;
  destroyEngine();
  showDebug(`wasm audio indisponible (${reason}) - repli AAC`);
  reportEvent("cast_receiver_wasm_fallback", reason);
  const url = stripWasmAudio(currentUrl);
  const request = new cast.framework.messages.LoadRequestData();
  request.media = new cast.framework.messages.MediaInformation();
  request.media.contentId = url;
  request.media.contentUrl = url;
  request.media.contentType = "application/x-mpegurl";
  request.media.hlsSegmentFormat = cast.framework.messages.HlsSegmentFormat.FMP4;
  request.media.hlsVideoSegmentFormat = cast.framework.messages.HlsVideoSegmentFormat.FMP4;
  request.currentTime = playerManager.getCurrentTimeSec() || 0;
  request.autoplay = true;
  playerManager.load(request).catch((e: unknown) => showDebug(`repli AAC impossible: ${String(e)}`));
}

// CAF's <video> element only exists once the first LOAD is being processed:
// at early start (as soon as we intercept), we poll for it.
async function findMediaElementWithRetry(timeoutMs: number): Promise<HTMLVideoElement | null> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const v = findMediaElement();
    if (v) return v;
    if (Date.now() > deadline) return null;
    await new Promise((r) => setTimeout(r, 250));
  }
}

// Started as soon as the LOAD is INTERCEPTED (not at PLAYER_LOAD_COMPLETE):
// the master fetch, the <video> wait and the wasm worker boot all run in
// parallel with the CAF load - that's what absorbs the initial seconds of
// silence. No desync risk: the engine only plays once the video ADVANCES
// (stall gate on the engine side), starting early just prepares things.
async function startWasmEngine(): Promise<void> {
  if (engine || engineStarting) return;
  engineStarting = true;
  const gen = loadGen;
  const url = currentUrl;
  const token = tokenFromUrl(url);
  try {
    const [video, masterText] = await Promise.all([
      findMediaElementWithRetry(10_000),
      fetch(url, { mode: "cors" }).then((r) => {
        if (!r.ok) throw new Error(`master ${r.status}`);
        return r.text();
      }),
    ]);
    if (gen !== loadGen) return; // another LOAD happened in the meantime
    if (!video) {
      fallbackToMuxedAudio("video element introuvable");
      return;
    }
    const audioUri = extractAudioURI(masterText);
    if (!audioUri) {
      // The server did not take the wasm path (non-Dolby track): the master
      // served is already muxed AAC, nothing to do.
      wasmRequested = false;
      return;
    }
    const playlistUrl = new URL(audioUri, url).toString();
    engine = new WasmAudioEngine(video, playlistUrl, {
      // ABSOLUTE resolution against the media server: the receiver can be
      // hosted elsewhere (GitHub Pages, project domain). And credentials
      // omit: the server's CORS token-auth does not answer Allow-Credentials.
      resolveUrl: (u) => absoluteWithToken(u, playlistUrl, token),
      credentials: "omit",
      // Audio-lead compensation: a TV's display pipeline delays the picture
      // much more than the engine's 0.1s desktop figure - measured ~0.5-0.6s
      // of audio lead on the Mi TV (2026-07-25). Value to calibrate from user
      // feedback: increase if the sound stays ahead, decrease if it swings
      // behind.
      leadCompensationSec: 0.65,
      onFatal: () => fallbackToMuxedAudio("moteur ac3go"),
    });
    await engine.start();
    showDebug("");
  } catch (err) {
    if (gen === loadGen) fallbackToMuxedAudio(String(err));
  } finally {
    engineStarting = false;
  }
}

playerManager.setMessageInterceptor(
  cast.framework.messages.MessageType.LOAD,
  (request: any) => {
    try {
      return interceptLoad(request);
    } catch (err) {
      // An exception here would silently fail the LOAD (905 with no network).
      // We show it ON SCREEN and let the request through INTACT: the mobile
      // sender's URL is already AAC-negotiated - the path that works.
      showDebug(`interceptor: ${String(err).slice(0, 160)}`);
      wasmRequested = false;
      currentUrl = request?.media?.contentUrl || request?.media?.contentId || "";
      return request;
    }
  },
);

function interceptLoad(request: any): any {
    loadGen++;
    destroyEngine();
    cancelEndShutdown();
    showDebug("");
    const media = request.media;
    if (!media) return request;
    const rawUrl: string = media.contentUrl || media.contentId || "";

    const deviceCaps = context.getDeviceCapabilities() ?? {};
    const systemHdr = deviceCaps[cast.framework.system.DeviceCapabilities.IS_HDR_SUPPORTED] === true;
    const probed = probeReceiverCaps(
      (mime, codec, w, h) => context.canDisplayType(mime, codec, w, h),
      systemHdr,
    );
    // v1: ZERO Dolby passthrough, even if the probe says yes - Jellyfin's
    // position (hasEAC3Support hardcoded false, "problems getting it to work
    // with HLS"), confirmed on the evening of 07-25: the Mi TV probes itself
    // Dolby-capable via EDID and the CAF player still fails the LOAD (code
    // 905) on the ec-3 master. So audio goes through ac3go wasm (https) or
    // the server's AAC transcode.
    const caps = { ...probed, nativeDolby: false };
    const wasmCapable = typeof WebAssembly !== "undefined" && !wasmDisabled;
    // window.location.protocol: an http: instance on the LAN is not allowed
    // JS fetches (mixed content) - the wasm decision accounts for this.
    const rewritten = rewriteCastMasterUrl(rawUrl, caps, wasmCapable, window.location.protocol);
    if (rewritten) {
      media.contentId = rewritten.url;
      media.contentUrl = rewritten.url;
      media.contentType = "application/x-mpegurl";
      // Explicit fMP4: without these hints the receiver assumes MPEG2-TS and
      // loads forever with no picture (same fix as the web/mobile sender).
      media.hlsSegmentFormat = cast.framework.messages.HlsSegmentFormat.FMP4;
      media.hlsVideoSegmentFormat = cast.framework.messages.HlsVideoSegmentFormat.FMP4;
      currentUrl = rewritten.url;
      wasmRequested = rewritten.wasmAudio;
      // EARLY start of the audio engine, in parallel with the CAF load.
      if (wasmRequested) void startWasmEngine();
    } else {
      currentUrl = rawUrl;
      wasmRequested = false;
    }
    return request;
}

// Safety net: if the early start did not succeed (late video element...),
// retry once the CAF load has finished - the old trigger.
playerManager.addEventListener(cast.framework.events.EventType.PLAYER_LOAD_COMPLETE, () => {
  if (wasmRequested && !engine) void startWasmEngine();
});

// Player errors show ON SCREEN: this is the debug receiver the DMR never was.
// detailedErrorCode + the raw reason + the negotiation decision: everything
// needed to diagnose from a photo of the TV.
playerManager.addEventListener(cast.framework.events.EventType.ERROR, (event: any) => {
  const code = event?.detailedErrorCode ?? "?";
  let extra = "";
  try {
    extra = JSON.stringify(event?.error ?? event?.reason ?? {}).slice(0, 140);
  } catch { /* the screen stays the channel: never throw here */ }
  const mode = wasmRequested ? "wasm" : "aac";
  const host = (() => { try { return new URL(currentUrl).host; } catch { return "?"; } })();
  showDebug(`Erreur lecture code=${code} mode=${mode} src=${host} ${extra}`);
  reportEvent("cast_receiver_error", `code=${code} mode=${mode} ${extra}`, true);
});

// Every way a playback can END, with its reason: END_OF_STREAM is the only
// normal one - ERROR/STOPPED/INTERRUPTED at an unexpected position is exactly
// the invisible TV-side death being chased.
playerManager.addEventListener(cast.framework.events.EventType.MEDIA_FINISHED, (event: any) => {
  const reason = event?.endedReason ?? "?";
  aliveHeartbeat.stop(); // plus rien ne joue : le battement mentirait
  reportEvent("cast_receiver_finished", `reason=${reason}`, reason !== "END_OF_STREAM");
});

playerManager.addEventListener(cast.framework.events.EventType.PLAYER_LOAD_COMPLETE, () => {
  reportEvent("cast_receiver_loaded", `mode=${wasmRequested ? "wasm" : "native"}`);
  aliveHeartbeat.start();
});

// A sender vanishing (phone frozen/killed by its OEM) must NOT stop playback
// per the Cast contract - this beacon timestamps the disconnect so the gap to
// any later death is measurable in the logs.
context.addEventListener(cast.framework.system.EventType.SENDER_DISCONNECTED, (event: any) => {
  let remaining = -1;
  try {
    remaining = context.getSenders()?.length ?? -1;
  } catch { /* count is best-effort */ }
  reportEvent("cast_receiver_sender_gone", `reason=${event?.reason ?? "?"} senders=${remaining}`);
});

context.addEventListener(cast.framework.system.EventType.SHUTDOWN, () => {
  aliveHeartbeat.stop();
  reportEvent("cast_receiver_shutdown", "receiver shutdown");
  destroyEngine();
});

// Dernier filet : la plateforme peut demonter la page sans passer par SHUTDOWN
// (c'est l'hypothese qui reste pour la mort invisible du 07-28). pagehide est
// l'evenement que les agents garantissent encore utilisable pour un
// sendBeacon, la ou unload ne l'est plus.
if (typeof window !== "undefined") {
  window.addEventListener("pagehide", () => {
    aliveHeartbeat.stop();
    reportEvent("cast_receiver_shutdown", "pagehide");
  });
}

// Belt and suspenders against the 2026-07-26 death (TV stops 3-5 min after
// the phone freezes, while the Cast contract says an implicit sender loss
// must not stop playback):
// - maxInactivity 1800s: a FROZEN sender (OEM freeze, WiFi asleep) stops
//   heartbeating without closing its socket; the default (10s) then reports
//   the sender lost within seconds and arms whatever teardown follows.
//   Jellyfin's own receiver documents the effect ("it quits once the client
//   closes the connection").
// - disableIdleTimeout: if playback silently IDLEs, stay up (with the debug
//   overlay + telemetry) instead of a silent teardown minutes later. A
//   genuine END_OF_STREAM closes the receiver explicitly below.
const receiverOptions = new cast.framework.CastReceiverOptions();
receiverOptions.maxInactivity = 1800;
receiverOptions.disableIdleTimeout = true;

// With the idle timeout disabled, a finished film would leave the receiver
// up forever: close it ourselves after a grace window (long enough for a
// sender to chain the next episode).
const END_SHUTDOWN_GRACE_MS = 60_000;
playerManager.addEventListener(cast.framework.events.EventType.MEDIA_FINISHED, (event: any) => {
  if (event?.endedReason !== "END_OF_STREAM") return;
  cancelEndShutdown();
  endShutdownTimer = setTimeout(() => {
    try {
      context.stop();
    } catch { /* platform already tearing down */ }
  }, END_SHUTDOWN_GRACE_MS);
});
playerManager.setMessageInterceptor(
  cast.framework.messages.MessageType.PLAY,
  (request: any) => {
    cancelEndShutdown();
    return request;
  },
);

context.start(receiverOptions);
