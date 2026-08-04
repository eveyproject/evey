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
import { extractAacAudioURI, extractAudioURI } from "@/features/playback/wasmaudio/wasmAudioShared";
import { absoluteWithToken, appendMaxKbps, bitrateTestUrl, buildPlayerEventUrl, createStartGate, createStarvationWatch, floorMasterUrl, probeMaxAudioChannels, probeReceiverCaps, rewriteCastMasterUrl, shouldMeasure, stereoFallbackUrl, stripWasmAudio } from "./receiverLogic";
import { createAliveHeartbeat, postPlayerEvent, postPlayerEventQuery, readHeapMB } from "./receiverBeacon";

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

// TV-side wasm audio kill-switch. OFF 2026-07-28 while the deployed decoder
// was the leaking TinyGo build (+6.75MB pinned per segment, box dead at
// ~3-4min); back ON 2026-07-29 with the standard-Go build (flat memory in the
// differential harness) and the bi-source relay (native AAC sound while the
// wasm boots). The mem= beacon on every alive heartbeat is the watch: a heap
// climbing beacon after beacon means flip this OFF again.
const CAST_WASM_ENABLED = true;

let engine: WasmAudioEngine | null = null;
let wasmDisabled = false;
let currentUrl = "";
let wasmRequested = false;
// Latch for the stereo retry (see fallbackToStereoAudio): one reload, never a loop.
let stereoForced = false;
// Answered by the device itself, once, at startup - probeMaxAudioChannels is
// async and the LOAD interceptor is not, so it cannot be asked inline. Stereo
// until it answers: the value that plays on every receiver, and the first LOAD
// arrives long after page load anyway.
let probedAudioChannels = 2;
void probeMaxAudioChannels({
  maxChannelCount: (() => {
    try {
      const Ctor = window.AudioContext ?? (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
      // Read the sink and close it again: this is a capability question, not
      // a reason to hold an audio context open for the whole session.
      if (!Ctor) return undefined;
      const ctx = new Ctor();
      const n = ctx.destination.maxChannelCount;
      void ctx.close();
      return n;
    } catch {
      return undefined;
    }
  })(),
  decodingInfo: navigator.mediaCapabilities
    ? (config) => navigator.mediaCapabilities.decodingInfo(config as MediaDecodingConfiguration)
    : undefined,
})
  .then((n) => {
    probedAudioChannels = n;
    reportEvent("cast_receiver_audio_channels", `probed=${n}`);
  })
  .catch(() => { /* stereo stands */ });
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
// Start gate: hold the picture until the audio engine has its first decoded
// segment, so the cast starts sound+image TOGETHER (the video-only master
// gives CAF nothing to wait for; without this the first 1-2s play silent).
// The 4s timeout means a broken audio path degrades to the old behaviour.
const startGate = createStartGate(() => {
  try {
    playerManager.play();
  } catch { /* torn down */ }
}, 4000);

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
// applyAdminCommand executes the admin panel's remote command ON the TV -
// the phone used to be the only consumer, so a frozen phone was an
// uncontrollable cast. Vocabulary mirrors the phone's consume path.
function applyAdminCommand(cmd: string): void {
  try {
    if (cmd === "pause") {
      playerManager.pause();
    } else if (cmd === "play") {
      playerManager.play();
    } else if (cmd === "stop") {
      context.stop();
    } else if (cmd.startsWith("message:")) {
      showDebug(`Message: ${cmd.slice(8)}`);
      setTimeout(() => showDebug(""), 12_000);
    }
  } catch {
    /* a command must never kill the receiver */
  }
}

const aliveHeartbeat = createAliveHeartbeat(() => {
  // mem: JS heap in MB (-1 if unavailable). A heap climbing beacon after
  // beacon toward the death names a memory kill; a flat one exonerates it.
  // mode: the engine's live relay leg (aac / wasm / aac-latched) when the
  // engine runs, else native CAF playback.
  const mode = engine ? engine.mode() : wasmRequested ? "wasm" : "native";
  let posSec = 0;
  try {
    posSec = playerManager.getCurrentTimeSec() || 0;
  } catch { /* player torn down already */ }
  // The alive beat rides fetch (not sendBeacon): its RESPONSE carries the
  // pending admin command. The death events keep sendBeacon - only they race
  // the page teardown.
  void postPlayerEventQuery(
    buildPlayerEventUrl(currentUrl),
    { type: "cast_receiver_alive", details: `mode=${mode} mem=${readHeapMB()} wasm=${engine?.decoderMemoryMB() ?? -1}`, posSec },
    (u, init) => fetch(u, init),
  ).then((cmd) => {
    if (cmd) applyAdminCommand(cmd);
  });
});

// Starvation rescue: the master advertises a low rung, but CAF never dives
// onto it by itself (Bouygtel4K 2026-08-03: variant pinned through nine
// minutes of freeze). This watch samples the playhead every 5s and, on
// sustained famine, reloads the SAME position on the ?abr=floor master - the
// server answers with only the lowest transcoded rung. Latched: the floored
// URL yields no further floor.
let starvationTimer: ReturnType<typeof setInterval> | null = null;

function stopStarvationWatch(): void {
  if (starvationTimer) {
    clearInterval(starvationTimer);
    starvationTimer = null;
  }
}

function startStarvationWatch(): void {
  stopStarvationWatch();
  const watch = createStarvationWatch();
  starvationTimer = setInterval(() => {
    let pos = 0;
    let state = "";
    try {
      pos = playerManager.getCurrentTimeSec() || 0;
      state = String(playerManager.getPlayerState() ?? "");
    } catch {
      return; // player torn down: the stop hooks will clear us
    }
    if (watch.sample(pos, state)) engageFloor(pos);
  }, 5_000);
}

function engageFloor(posSec: number): void {
  const url = floorMasterUrl(currentUrl);
  if (!url) return; // already floored, or a URL we don't own: nothing lower
  reportEvent("cast_receiver_floor_rescue", `starvation at pos=${posSec.toFixed(1)}s - reloading on the floor rung`);
  showDebug("Débit insuffisant - passage en qualité réduite");
  setTimeout(() => showDebug(""), 8_000);
  startGate.cancel();
  destroyEngine();
  const request = new cast.framework.messages.LoadRequestData();
  request.media = new cast.framework.messages.MediaInformation();
  request.media.contentId = url;
  request.media.contentUrl = url;
  request.media.contentType = "application/x-mpegurl";
  request.media.hlsSegmentFormat = cast.framework.messages.HlsSegmentFormat.FMP4;
  request.media.hlsVideoSegmentFormat = cast.framework.messages.HlsVideoSegmentFormat.FMP4;
  request.currentTime = posSec;
  request.autoplay = true;
  playerManager.load(request).catch((e: unknown) => showDebug(`qualité réduite impossible: ${String(e)}`));
}

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
  startGate.cancel(); // the AAC reload below carries its own autoplay
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

// Fallback for a device that ANSWERED 5.1 and then refused the stream: reload
// the same position with the ceiling pinned to stereo. The Mi TV already lied
// once through EDID (2026-07-25), so the probe gets a net rather than blind
// trust. stereoForced is a latch - one retry, and every later LOAD stays
// stereo for this session.
function fallbackToStereoAudio(reason: string): void {
  if (stereoForced) return;
  const url = stereoFallbackUrl(currentUrl);
  if (!url) return;
  stereoForced = true;
  currentUrl = url;
  showDebug(`audio 5.1 refuse (${reason}) - repli stereo`);
  reportEvent("cast_receiver_stereo_fallback", reason);
  const request = new cast.framework.messages.LoadRequestData();
  request.media = new cast.framework.messages.MediaInformation();
  request.media.contentId = url;
  request.media.contentUrl = url;
  request.media.contentType = "application/x-mpegurl";
  request.media.hlsSegmentFormat = cast.framework.messages.HlsSegmentFormat.FMP4;
  request.media.hlsVideoSegmentFormat = cast.framework.messages.HlsVideoSegmentFormat.FMP4;
  request.currentTime = playerManager.getCurrentTimeSec() || 0;
  request.autoplay = true;
  playerManager.load(request).catch((e: unknown) => showDebug(`repli stereo impossible: ${String(e)}`));
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
      // served is already muxed AAC, nothing to do - and nothing to hold the
      // start for.
      wasmRequested = false;
      startGate.skip();
      return;
    }
    const playlistUrl = new URL(audioUri, url).toString();
    // Bi-source relay: the AAC rendition (native decodeAudioData) carries the
    // first seconds - on a TV's CPU the Go-wasm compile is the dominant cost
    // of time-to-sound - then the engine hands over to AC-3 + wasm at a
    // segment boundary. Tag absent (older server) = wasm-only, as before.
    const aacUri = extractAacAudioURI(masterText);
    engine = new WasmAudioEngine(video, playlistUrl, {
      // ABSOLUTE resolution against the media server: the receiver can be
      // hosted elsewhere (GitHub Pages, project domain). And credentials
      // omit: the server's CORS token-auth does not answer Allow-Credentials.
      resolveUrl: (u) => absoluteWithToken(u, playlistUrl, token),
      credentials: "omit",
      aacPlaylistUrl: aacUri ? new URL(aacUri, url).toString() : undefined,
      // Every relay transition lands in the server logs: the next cast names
      // its audio path (aac -> wasm, or aac-latched) without a TV screenshot.
      onModeChange: (mode) => reportEvent("cast_receiver_audio_mode", mode),
      // WHY the relay latched: a refused hand-over and a starvation fallback
      // both read "aac-latched", and telling them apart from prod logs alone
      // cost hours on 2026-07-30.
      onDiagnostic: (detail) => reportEvent("cast_receiver_audio_guard", detail),
      // Releases the start gate: picture and sound begin together.
      onFirstAudio: () => startGate.audioReady(),
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

// measureThroughputKbps times the receiver's own stack against the server's
// bitrate-test bytes: read the stream for up to ~2.5s, compute kbps from what
// actually arrived. Null (no verdict) on any failure or too little signal -
// the load then proceeds exactly as before the measure existed.
async function measureThroughputKbps(masterUrl: string): Promise<number | null> {
  const testUrl = bitrateTestUrl(masterUrl);
  if (!testUrl) return null;
  try {
    const ctrl = new AbortController();
    const hardStop = setTimeout(() => ctrl.abort(), 3500);
    const started = Date.now();
    const resp = await fetch(testUrl, { mode: "cors", signal: ctrl.signal });
    if (!resp.ok || !resp.body) {
      clearTimeout(hardStop);
      return null;
    }
    const reader = resp.body.getReader();
    let bytes = 0;
    for (;;) {
      const { done, value } = await reader.read();
      if (value) bytes += value.byteLength;
      const elapsedMs = Date.now() - started;
      if (done || elapsedMs >= 2500) {
        if (!done) ctrl.abort();
        clearTimeout(hardStop);
        // Under 200ms or 100KB the sample says nothing usable.
        if (elapsedMs < 200 || bytes < 100_000) return null;
        return Math.round((bytes * 8) / elapsedMs); // bits per ms = kbps
      }
    }
  } catch {
    return null;
  }
}

playerManager.setMessageInterceptor(
  cast.framework.messages.MessageType.LOAD,
  (request: any) => {
    try {
      // Entry measure, BEFORE the load proceeds (CAF holds the LOAD on a
      // returned promise): the server then fits the master to the verdict
      // (?maxKbps=, copy-first). A rescue reload or a foreign URL skips it.
      const rawUrl: string = request?.media?.contentUrl || request?.media?.contentId || "";
      if (request?.media && shouldMeasure(rawUrl)) {
        return measureThroughputKbps(rawUrl)
          .then((kbps) => {
            const fitted = kbps ? appendMaxKbps(rawUrl, kbps) : null;
            if (fitted) {
              request.media.contentUrl = fitted;
              request.media.contentId = fitted;
            }
            // interceptLoad first: it sets currentUrl, which carries the
            // beacon's token - reported after, the measure lands in the logs.
            const out = interceptLoad(request);
            if (kbps) reportEvent("cast_receiver_bitrate_measure", `kbps=${kbps}`);
            return out;
          })
          .catch(() => interceptLoad(request));
      }
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
    startGate.cancel(); // a new LOAD carries its own autoplay
    destroyEngine();
    cancelEndShutdown();
    // The load transition reads as BUFFERING: never let the old watch count it.
    // PLAYER_LOAD_COMPLETE re-arms a fresh one.
    stopStarvationWatch();
    showDebug("");
    const media = request.media;
    if (!media) return request;
    const rawUrl: string = media.contentUrl || media.contentId || "";

    const deviceCaps = context.getDeviceCapabilities() ?? {};
    const systemHdr = deviceCaps[cast.framework.system.DeviceCapabilities.IS_HDR_SUPPORTED] === true;
    const probed = probeReceiverCaps(
      (mime, codec, w, h) => context.canDisplayType(mime, codec, w, h),
      systemHdr,
      stereoForced ? 2 : probedAudioChannels,
    );
    // v1: ZERO Dolby passthrough, even if the probe says yes - Jellyfin's
    // position (hasEAC3Support hardcoded false, "problems getting it to work
    // with HLS"), confirmed on the evening of 07-25: the Mi TV probes itself
    // Dolby-capable via EDID and the CAF player still fails the LOAD (code
    // 905) on the ec-3 master. So audio goes through ac3go wasm (https) or
    // the server's AAC transcode.
    const caps = { ...probed, nativeDolby: false };
    // wasm off on cast (2026-07-28): every wasm cast memory-died at 3-4 min
    // on 1-2GB TV boxes. Jellyfin policy instead - the TV only gets codecs it
    // decodes natively, audio rides the server's AAC rendition.
    const wasmCapable = CAST_WASM_ENABLED && typeof WebAssembly !== "undefined" && !wasmDisabled;
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
      if (wasmRequested) {
        // Hold the visible start until the engine's first decoded segment
        // (or the gate's timeout): sound and picture begin together.
        request.autoplay = false;
        startGate.arm();
        // EARLY start of the audio engine, in parallel with the CAF load.
        void startWasmEngine();
      }
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
  // On the AAC path the wasm fallback has nothing to offer; a refused 5.1
  // layout does. The wasm path keeps its own fallback (fallbackToMuxedAudio).
  if (!wasmRequested) fallbackToStereoAudio(`code=${code}`);
});

// Every way a playback can END, with its reason: END_OF_STREAM is the only
// normal one - ERROR/STOPPED/INTERRUPTED at an unexpected position is exactly
// the invisible TV-side death being chased.
playerManager.addEventListener(cast.framework.events.EventType.MEDIA_FINISHED, (event: any) => {
  const reason = event?.endedReason ?? "?";
  aliveHeartbeat.stop(); // plus rien ne joue : le battement mentirait
  stopStarvationWatch();
  reportEvent("cast_receiver_finished", `reason=${reason}`, reason !== "END_OF_STREAM");
});

playerManager.addEventListener(cast.framework.events.EventType.PLAYER_LOAD_COMPLETE, () => {
  reportEvent("cast_receiver_loaded", `mode=${wasmRequested ? "wasm" : "native"}`);
  aliveHeartbeat.start();
  startStarvationWatch();
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
  stopStarvationWatch();
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
    stopStarvationWatch();
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
