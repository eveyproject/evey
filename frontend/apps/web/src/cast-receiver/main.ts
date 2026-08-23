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
import { absoluteWithToken, appendMaxKbps, bitrateTestUrl, buildPlayerEventUrl, bufferedStats, createStartGate, createStartWatch, createStarvationWatch, describePlayerError, planBackBufferTrim, planFloorRescue, probeMaxAudioChannels, probeReceiverCaps, rewriteCastMasterUrl, shouldMeasure, stereoFallbackUrl, stripWasmAudio } from "./receiverLogic";
import { createAliveHeartbeat, createFrameDropTracker, formatFrameQuality, postPlayerEvent, postPlayerEventQuery, readHeapMB } from "./receiverBeacon";
import { buildCastNextUrl, buildNextMasterUrl, parseCastNextResponse, shouldFetchNext, upNextLabel, type CastNextInfo } from "./castNext";
import { buildSegmentsUrl, introSkippedLabel, parseSegments, planIntroSkip, type SkippableSegment } from "./introSkip";
import { endCardModel, pauseHudModel } from "./hud";

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

// appendDebug adds a line instead of replacing the panel. A fallback that
// overwrote the message left only its own outcome on screen: on 2026-08-08 the
// TV showed "repli stereo impossible" and the ERROR code that caused the
// fallback in the first place was gone - the one thing a photo of the TV had
// to carry.
function appendDebug(text: string): void {
  if (!dbg || !text) return;
  const current = dbg.textContent ?? "";
  showDebug(current ? `${current}\n${text}` : text);
}

// TV-side wasm audio kill-switch. OFF 2026-07-28 while the deployed decoder
// was the leaking TinyGo build (+6.75MB pinned per segment, box dead at
// ~3-4min); back ON 2026-07-29 with the standard-Go build (flat memory in the
// differential harness) and the bi-source relay (native AAC sound while the
// wasm boots). The mem= beacon on every alive heartbeat is the watch: a heap
// climbing beacon after beacon means flip this OFF again.
const CAST_WASM_ENABLED = true;

// Shaka for HLS (what jellyfin-chromecast ships): eviction, buffer goal and
// gap jumping are native there; MPL raises no ERROR, wedges on skipped
// segments and has no buffer control. false = back to MPL.
const CAST_USE_SHAKA_FOR_HLS = true;
// Buffer bounds sized for a 2GB box decoding a ~10 Mbps copy.
const SHAKA_BUFFER_BEHIND_SEC = 30;
const SHAKA_BUFFERING_GOAL_SEC = 20;

// Manual back-buffer trim, MPL fallback only: Shaka evicts by itself.
const CAST_BUFFER_TRIM_ENABLED = true;
const TRIM_KEEP_BEHIND_SEC = 20;
const TRIM_MIN_SPAN_SEC = 10;
const TRIM_INTERVAL_MS = 15_000;

// The player owns its MediaSource: capturing addSourceBuffer is the only handle.
const trackedSourceBuffers = new Set<SourceBuffer>();
if (typeof MediaSource !== "undefined") {
  const origAddSourceBuffer = MediaSource.prototype.addSourceBuffer;
  MediaSource.prototype.addSourceBuffer = function (mime: string): SourceBuffer {
    const sb = origAddSourceBuffer.call(this, mime);
    trackedSourceBuffers.add(sb);
    return sb;
  };
}

function toRanges(tr: TimeRanges): Array<[number, number]> {
  const out: Array<[number, number]> = [];
  for (let i = 0; i < tr.length; i++) out.push([tr.start(i), tr.end(i)]);
  return out;
}

let trimCount = 0;
let trimTimer: ReturnType<typeof setInterval> | null = null;

function stopBufferTrim(): void {
  if (trimTimer) {
    clearInterval(trimTimer);
    trimTimer = null;
  }
}

function startBufferTrim(): void {
  stopBufferTrim();
  if (!CAST_BUFFER_TRIM_ENABLED || CAST_USE_SHAKA_FOR_HLS) return;
  trimCount = 0;
  trimTimer = setInterval(() => {
    const video = findMediaElement();
    if (!video || video.paused) return;
    const t = video.currentTime;
    for (const sb of trackedSourceBuffers) {
      try {
        // never race the player's own operation
        if (sb.updating) continue;
        const plan = planBackBufferTrim(toRanges(sb.buffered), t, TRIM_KEEP_BEHIND_SEC, TRIM_MIN_SPAN_SEC);
        if (plan) {
          sb.remove(plan.start, plan.end);
          trimCount++;
        }
      } catch {
        trackedSourceBuffers.delete(sb); // detached: its load is gone
      }
    }
  }, TRIM_INTERVAL_MS);
}

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
// The probe resolves at page load, when currentUrl is still empty - a beacon
// sent then has no token and is dropped silently (2026-08-05: prod showed
// zero cast_receiver_audio_channels ever, and the absence read as "old bundle
// cached" during an incident). Reported at the first LOAD instead, with the
// value that load actually used.
let audioChannelsReported = false;
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

// Start watch: a LOAD that never reaches PLAYER_LOAD_COMPLETE is declared
// dead and reloaded ONCE on the floor master (mono-variant, muxed - the shape
// the Chromecast HD provably plays; its stall on the hybrid master raised no
// ERROR at all, so this deadline is the only net that can catch it). 12s: a
// healthy cast load completes in ~3-5s on these boxes, bitrate measure
// included; the observed deaths sat past 100s. The rescue's own reload
// re-arms through interceptLoad - if the floor stalls too, floorMasterUrl's
// latch (null on an already-floored URL) ends the chain.
const startWatch = createStartWatch(() => {
  engageFloor(0, "load never completed");
}, 12_000);

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

// Frames perdues par le decodeur de la box, d'un battement a l'autre. Le
// 2026-08-23 un cast a gele a tampon PLEIN : toutes les mesures serveur
// exoneraient la livraison, celle-ci manquait.
const frameDrops = createFrameDropTracker();

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
  // buf=behind/ahead/ranges (ranges>1 = a hole); mem/wasm are blind to MSE.
  let buf = "-";
  const video = findMediaElement();
  if (video) {
    try {
      const s = bufferedStats(toRanges(video.buffered), video.currentTime);
      buf = `${s.behindSec.toFixed(0)}/${s.aheadSec.toFixed(0)}/${s.rangeCount}`;
    } catch { /* buffered can throw mid-teardown */ }
  }
  const drops = formatFrameQuality(frameDrops.sample(video));
  // The alive beat rides fetch (not sendBeacon): its RESPONSE carries the
  // pending admin command. The death events keep sendBeacon - only they race
  // the page teardown.
  void postPlayerEventQuery(
    buildPlayerEventUrl(currentUrl),
    { type: "cast_receiver_alive", details: `mode=${mode} mem=${readHeapMB()} wasm=${engine?.decoderMemoryMB() ?? -1} buf=${buf} trim=${trimCount} ${drops}`, posSec },
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

// One in-place reload per floored playback: heals a wedge, cannot loop.
const MAX_FLOOR_RELOADS = 1;
let floorReloads = 0;

function engageFloor(posSec: number, reason = "starvation"): void {
  const plan = planFloorRescue(currentUrl, floorReloads, MAX_FLOOR_RELOADS);
  if (!plan) return; // not a URL we own
  if (plan.kind === "exhausted") {
    reportEvent("cast_receiver_floor_rescue", `${reason} at pos=${posSec.toFixed(1)}s - floor exhausted, no rescue left`, true);
    return;
  }
  if (plan.kind === "floor-reload") floorReloads++;
  const url = plan.url;
  reportEvent(
    "cast_receiver_floor_rescue",
    plan.kind === "floor-reload"
      ? `${reason} at pos=${posSec.toFixed(1)}s - reloading the floor in place`
      : `${reason} at pos=${posSec.toFixed(1)}s - reloading on the floor rung`,
  );
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

// --- Next-episode auto-advance (logic in castNext.ts) ---------------------
// Near the end of an episode the receiver asks the server what comes next
// (cast-next, authenticated by its own stream token - the server prewarns the
// first segments on that call), and at END_OF_STREAM it announces the episode
// with a short countdown then self-loads it. The phone can stay asleep.
const AUTO_ADVANCE_COUNTDOWN_SEC = 5;
let nextInfo: CastNextInfo | null = null;
let nextFetchAttempted = false;
let nextWatchTimer: ReturnType<typeof setInterval> | null = null;
let upNextTimer: ReturnType<typeof setInterval> | null = null;

const upnextEl = document.getElementById("upnext");
function showUpNext(heading: string, title: string, count: string): void {
  if (!upnextEl) return;
  const set = (cls: string, text: string) => {
    const el = upnextEl.querySelector(`.${cls}`);
    if (el) el.textContent = text;
  };
  set("heading", heading);
  set("title", title);
  set("count", count);
  upnextEl.style.display = "block";
}
function hideUpNext(): void {
  if (upNextTimer) {
    clearInterval(upNextTimer);
    upNextTimer = null;
  }
  if (upnextEl) upnextEl.style.display = "none";
}

function resetNextEpisode(): void {
  nextInfo = null;
  nextFetchAttempted = false;
  stopNextWatch();
  hideUpNext();
}

function stopNextWatch(): void {
  if (nextWatchTimer) {
    clearInterval(nextWatchTimer);
    nextWatchTimer = null;
  }
}

function startNextWatch(): void {
  stopNextWatch();
  nextWatchTimer = setInterval(() => {
    let pos = 0;
    let dur = 0;
    try {
      pos = playerManager.getCurrentTimeSec() || 0;
      dur = playerManager.getDurationSec() || 0;
    } catch {
      return; // player torn down: the stop hooks will clear us
    }
    if (shouldFetchNext(pos, dur, nextFetchAttempted)) void fetchNextEpisode();
  }, 10_000);
}

async function fetchNextEpisode(): Promise<void> {
  nextFetchAttempted = true;
  const url = buildCastNextUrl(currentUrl);
  if (!url) return;
  try {
    const resp = await fetch(url, { method: "POST", mode: "cors", credentials: "omit" });
    if (!resp.ok || resp.status === 204) return;
    nextInfo = parseCastNextResponse(await resp.json());
    if (nextInfo) reportEvent("cast_receiver_next_available", `next=${nextInfo.nextMediaItemId}`);
  } catch {
    nextInfo = null; // chaining is a nicety, never a failure path
  }
}

async function autoAdvance(): Promise<void> {
  // A viewer who seeked straight into the credits can outrun the 10s watch.
  if (!nextInfo && !nextFetchAttempted) await fetchNextEpisode();
  const info = nextInfo;
  if (!info) {
    // Finale/movie/denied: nothing to chain - dress the shutdown grace
    // instead of leaving a black screen until context.stop().
    showEndCard();
    return;
  }
  const target = buildNextMasterUrl(currentUrl, info);
  if (!target) {
    showEndCard();
    return;
  }
  cancelEndShutdown();
  const label = upNextLabel(info, typeof navigator !== "undefined" ? navigator.language ?? "en" : "en");
  let remaining = AUTO_ADVANCE_COUNTDOWN_SEC;
  if (upNextTimer) {
    clearInterval(upNextTimer);
    upNextTimer = null;
  }
  showUpNext(label.heading, label.title, String(remaining));
  upNextTimer = setInterval(() => {
    remaining--;
    if (remaining > 0) {
      showUpNext(label.heading, label.title, String(remaining));
      return;
    }
    hideUpNext();
    reportEvent("cast_receiver_next_episode", `next=${info.nextMediaItemId}`);
    const request = new cast.framework.messages.LoadRequestData();
    request.media = new cast.framework.messages.MediaInformation();
    request.media.contentId = target;
    request.media.contentUrl = target;
    request.media.contentType = "application/x-mpegurl";
    request.media.hlsSegmentFormat = cast.framework.messages.HlsSegmentFormat.FMP4;
    request.media.hlsVideoSegmentFormat = cast.framework.messages.HlsVideoSegmentFormat.FMP4;
    request.media.metadata = new cast.framework.messages.GenericMediaMetadata();
    request.media.metadata.title = info.title || label.title;
    request.autoplay = true;
    playerManager.load(request).catch((e: unknown) => reportFallbackFailure("episode suivant", e));
  }, 1_000);
}

// --- Intro auto-skip (logic in introSkip.ts) ------------------------------
// The web player's "skip intro" button has no TV equivalent (no input), so
// the receiver jumps detected intros itself, with a 3s notice. One skip per
// segment per load: seeking back into the intro means watching it.
let introSegments: SkippableSegment[] = [];
const introSkipped = new Set<number>();
let introWatchTimer: ReturnType<typeof setInterval> | null = null;

function resetIntroSkip(): void {
  introSegments = [];
  introSkipped.clear();
  if (introWatchTimer) {
    clearInterval(introWatchTimer);
    introWatchTimer = null;
  }
}

async function fetchIntroSegments(): Promise<void> {
  const url = buildSegmentsUrl(currentUrl);
  if (!url) return;
  try {
    const resp = await fetch(url, { mode: "cors", credentials: "omit" });
    if (!resp.ok) return;
    introSegments = parseSegments(await resp.json());
    if (introSegments.length > 0) startIntroWatch();
  } catch {
    /* skipping is a nicety, never a failure path */
  }
}

function startIntroWatch(): void {
  if (introWatchTimer) return;
  // 1s cadence: an intro skipped 5s late defeats its purpose.
  introWatchTimer = setInterval(() => {
    let pos = 0;
    try {
      pos = playerManager.getCurrentTimeSec() || 0;
    } catch {
      return; // player torn down: the stop hooks will clear us
    }
    const seg = planIntroSkip(introSegments, pos, introSkipped);
    if (!seg) return;
    introSkipped.add(seg.startSec);
    try {
      playerManager.seek(seg.endSec);
    } catch {
      return; // seek refused: leave the segment unskipped-marked anyway, no loop
    }
    reportEvent("cast_receiver_intro_skipped", `from=${pos.toFixed(1)} to=${seg.endSec.toFixed(1)}`);
    showDebug(introSkippedLabel(typeof navigator !== "undefined" ? navigator.language ?? "en" : "en"));
    setTimeout(() => showDebug(""), 3_000);
  }, 1_000);
}

// --- Pause HUD + end card (models in hud.ts) ------------------------------
const hudEl = document.getElementById("hud");
const endcardEl = document.getElementById("endcard");

function deviceLang(): string {
  return typeof navigator !== "undefined" ? navigator.language ?? "en" : "en";
}

function currentTitle(): string {
  try {
    return String(playerManager.getMediaInformation()?.metadata?.title ?? "");
  } catch {
    return "";
  }
}

function setCardText(root: HTMLElement | null, values: Record<string, string>): void {
  if (!root) return;
  for (const [cls, text] of Object.entries(values)) {
    const el = root.querySelector(`.${cls}`);
    if (el) el.textContent = text;
  }
}

function showPauseHud(): void {
  if (!hudEl) return;
  let pos = 0;
  let dur = 0;
  try {
    pos = playerManager.getCurrentTimeSec() || 0;
    dur = playerManager.getDurationSec() || 0;
  } catch {
    return;
  }
  const hud = pauseHudModel(currentTitle(), pos, dur, deviceLang());
  setCardText(hudEl, { title: hud.title, clock: hud.clock, remaining: hud.remaining });
  hudEl.style.display = "block";
}

function hidePauseHud(): void {
  if (hudEl) hudEl.style.display = "none";
}

function showEndCard(): void {
  if (!endcardEl) return;
  const card = endCardModel(currentTitle(), deviceLang());
  setCardText(endcardEl, { heading: card.heading, title: card.title });
  endcardEl.style.display = "block";
}

function hideEndCard(): void {
  if (endcardEl) endcardEl.style.display = "none";
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
  appendDebug(`wasm audio indisponible (${reason}) - repli AAC`);
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
  playerManager.load(request).catch((e: unknown) => reportFallbackFailure("aac", e));
}

// reportFallbackFailure is the end of a rescue path: the reload we bet on was
// itself refused. Screen AND beacon, because a fallback that dies silently
// reads exactly like one that was never tried - the 2026-08-08 cast left no
// server-side trace of its failed stereo reload at all.
function reportFallbackFailure(kind: string, err: unknown): void {
  const detail = describePlayerError(err);
  appendDebug(`repli ${kind} impossible: ${detail}`);
  reportEvent("cast_receiver_fallback_failed", `kind=${kind} ${detail}`, true);
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
  appendDebug(`audio 5.1 refuse (${reason}) - repli stereo`);
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
  playerManager.load(request).catch((e: unknown) => reportFallbackFailure("stereo", e));
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
    stopBufferTrim();
    resetNextEpisode(); // the new media gets its own next-episode lookup
    resetIntroSkip();
    hidePauseHud();
    hideEndCard();
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
      startWatch.arm();
      if (!audioChannelsReported) {
        audioChannelsReported = true;
        reportEvent("cast_receiver_audio_channels", `probed=${stereoForced ? 2 : probedAudioChannels}`);
      }
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
    // A fresh (non-floored) cast restores the floor-reload budget.
    try {
      if (new URL(currentUrl).searchParams.get("abr") !== "floor") floorReloads = 0;
    } catch { /* foreign URL: the rescue never runs on it anyway */ }
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
    extra = describePlayerError(event?.error ?? event?.reason);
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
  stopBufferTrim();
  stopNextWatch();
  resetIntroSkip();
  startWatch.cancel();
  reportEvent("cast_receiver_finished", `reason=${reason}`, reason !== "END_OF_STREAM");
  // Natural end of an episode: announce and chain the next one. Any other
  // reason (STOPPED, ERROR, INTERRUPTED) must not resurrect a playback the
  // viewer or the platform just ended.
  if (reason === "END_OF_STREAM") void autoAdvance();
});

playerManager.addEventListener(cast.framework.events.EventType.PLAYER_LOAD_COMPLETE, () => {
  startWatch.loaded();
  reportEvent("cast_receiver_loaded", `mode=${wasmRequested ? "wasm" : "native"} engine=${CAST_USE_SHAKA_FOR_HLS ? "shaka" : "mpl"}`);
  aliveHeartbeat.start();
  startStarvationWatch();
  startBufferTrim();
  startNextWatch();
  void fetchIntroSegments();
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
  stopBufferTrim();
  resetNextEpisode();
  resetIntroSkip();
  startWatch.cancel();
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
    stopBufferTrim();
    resetNextEpisode();
    resetIntroSkip();
    startWatch.cancel();
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
receiverOptions.useShakaForHls = CAST_USE_SHAKA_FOR_HLS;
if (CAST_USE_SHAKA_FOR_HLS) {
  receiverOptions.playbackConfig = new cast.framework.PlaybackConfig();
  receiverOptions.playbackConfig.shakaConfig = {
    streaming: {
      bufferBehind: SHAKA_BUFFER_BEHIND_SEC,
      bufferingGoal: SHAKA_BUFFERING_GOAL_SEC,
    },
  };
}

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

// Pause HUD: what is paused, where it stands, what remains. The stock
// player's control bar fades and leaves a dark frame; this stays up for the
// whole pause. Refreshed on seek (the paused frame can move under a
// scrub from the phone), gone the instant playback resumes or ends.
playerManager.addEventListener(cast.framework.events.EventType.PAUSE, () => {
  showPauseHud();
});
playerManager.addEventListener(cast.framework.events.EventType.PLAY, () => {
  hidePauseHud();
});
playerManager.addEventListener(cast.framework.events.EventType.SEEKED, () => {
  try {
    if (String(playerManager.getPlayerState() ?? "") === "PAUSED") showPauseHud();
  } catch { /* torn down */ }
});
playerManager.addEventListener(cast.framework.events.EventType.MEDIA_FINISHED, () => {
  hidePauseHud();
});

context.start(receiverOptions);
