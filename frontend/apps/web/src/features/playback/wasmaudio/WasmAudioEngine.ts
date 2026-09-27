// WasmAudioEngine plays a video's AC-3 / E-AC-3 audio in a browser that cannot
// decode it natively (Chrome, Firefox, Edge). The video rides hls.js/MSE MUTED
// (video-only master); this engine fetches the AC-3 audio rendition the server
// named in the master's #EVEY-AUDIO-URI tag, decodes each segment to PCM in a
// Web Worker (the ac3go wasm decoder), and plays the samples through Web Audio,
// clock-locked to video.currentTime.
//
// Sync model: the video element is the master clock. An anchor pins a moment of
// the AudioContext clock to a moment of the media timeline; every decoded
// segment is scheduled at the AudioContext time its media start maps to (plus a
// small constant lead compensation). Both clocks freeze together on pause
// (AudioContext.suspend), so the anchor survives a pause untouched. A seek, or a
// drift past the resync threshold (stall recovery), re-anchors and re-fills from
// the new position - cheap because the decoder runs ~75x real time.
//
// Channels: a 5.1 stream is remapped from the decoder's L,C,R,Ls,Rs,LFE order to
// Web Audio's SMPTE L,R,C,LFE,Ls,Rs so the browser routes (and downmixes to
// stereo outputs) correctly; other channel counts pass straight through.

import { AVLatencyEstimator } from "./avLatencyEstimator";
import { shouldHandover, isStarved, owedSince, type HandoverVerdict } from "./relayGuard";
import { AUDIO_STUCK_MS, audioClockFrozen, healBackoffMs, stuckFor, type ClockSample } from "./audioHealth";
import { AAC_SAMPLES_PER_FRAME, adtsFromFrames, extractAacFrames, parseAacConfig, type AacConfig } from "./fmp4Adts";
import { outputMix } from "./channelMix";
import { planRelayHandover, type Segment } from "./relayHandover";
import { SegmentRetryTracker } from "./segmentRetry";
import { pcmIsSilent, SilenceRun } from "./silenceWatch";

const WASM_URL = "/wasm/ac3go.wasm";
const WASM_EXEC_URL = "/wasm/wasm_exec.js";
const WORKER_URL = "/wasm/ac3Worker.js";

// How far ahead of the playhead to keep audio decoded+scheduled. Generous: the
// decoder is far faster than real time, and a deep buffer rides out a stalled
// segment fetch without an audible gap.
const BUFFER_AHEAD_SEC = 20;
// How far past the look-ahead window a decoded segment may sit before it is
// dropped. The cache only ever evicted BEHIND the playhead, so seeking backward
// stranded everything decoded ahead: 35 segments on prod (2026-09-20), and a
// decoded 6 s 5.1 segment is ~7 MB of PCM. Generous enough that a small step
// back still lands in the cache; a real jump pays one re-decode.
const EVICT_AHEAD_MARGIN_SEC = 30;
// Re-anchor when the audio clock and the video clock disagree by more than this.
// Well above the natural ~0.1 ms/s drift (measured negligible) so only a seek or
// a stall recovery trips it - a re-anchor is a brief audio reset, not for free.
const RESYNC_THRESHOLD_SEC = 0.3;
// Constant lead compensation: the audio otherwise starts ~100 ms ahead of the
// picture (decoder has no output latency; the display + hls.js pipeline does).
// Delaying the audio by roughly the output latency lines them up. Tunable.
const DEFAULT_LEAD_COMP_SEC = 0.1;
const SCHEDULE_TICK_MS = 100;
// How many segments one tick may hand to Web Audio. See scheduleWindow: each
// one costs an AudioBuffer allocation (plus, for a cached old worker still on
// the interleaved protocol, a de-interleave) on the main thread, and doing the
// whole window at once is what makes the picture hitch.
const SCHEDULE_PER_TICK = 2;

// Two decode outputs: the wasm worker hands PCM in the decoder's channel
// order (remapped to the browser's order at schedule time); the native leg
// hands the AudioBuffer decodeAudioData built (already browser-ordered),
// plus the offset of the preroll samples to skip (see decodeNativeSegment).
//
// The worker's PCM reply comes in one of three shapes, newest first:
//   - planar + mixed: one Float32Array per OUTPUT channel, the mix table
//     already applied on the worker thread. Nothing but native copies remain
//     here, whatever the layout - 7.1 included.
//   - planar, not mixed: one array per SOURCE channel; the permutation is a
//     native copy, the 7.1 fold still sums on this thread.
//   - interleaved `pcm`: a cached old ac3Worker.js served against a fresh
//     page (see that file's protocol comment). De-interleaved here.
// All three are kept because the worker is a static file the browser caches
// on its own schedule.
export type PcmDecoded =
  | { kind: "pcm"; channels: number; sampleRate: number; frames: number; planar: Float32Array<ArrayBuffer>[]; mixed?: boolean }
  | { kind: "pcm"; channels: number; sampleRate: number; frames: number; pcm: Float32Array };

// workerMixTable is what the worker needs to hand back OUTPUT channels: the
// same outputMix, for every layout the decoder can produce, keyed by source
// channel count. Built here so channelMix.ts stays the single definition.
export function workerMixTable(): Record<number, Array<Array<[number, number]>>> {
  const table: Record<number, Array<Array<[number, number]>>> = {};
  for (let ch = 1; ch <= 8; ch++) table[ch] = outputMix(ch);
  return table;
}

type DecodedSegment = PcmDecoded | { kind: "buffer"; audioBuffer: AudioBuffer; offsetSec: number };

// SegmentHttpError carries the response status through the decode Promise
// chain so the pump loop can tell an auth failure (401/403 - the sliding
// stream token expired) apart from any other segment error, without the two
// decode paths (wasm fetch, native fetch) duplicating that logic.
class SegmentHttpError extends Error {
  constructor(readonly status: number) {
    super(`segment ${status}`);
    this.name = "SegmentHttpError";
  }
}

function isAuthStatus(err: unknown): boolean {
  return err instanceof SegmentHttpError && (err.status === 401 || err.status === 403);
}

// mayResumeAudioContext answers "is now a moment where this suspended context
// may be woken?" - the pure seam behind resumeCtx.
//
// `armed` is the one-shot retry latch (an autoplay-refused resume() disarms it
// so the 100ms tick can't spam the console until a gesture re-arms it).
//
// videoPaused is the half that was missing until 2026-09-19. A pause suspends
// the context and KEEPS its scheduled sources, which is what makes the resume
// seamless; waking it while the picture is still frozen plays those sources
// until the next tick suspends it again. The engine's gesture unlock listens
// on the window for its whole life, so every click anywhere in the page did
// exactly that: a tenth of a second of sound per click while paused ("le son
// se lance quelques millièmes de seconde à chaque clic" - browsing the
// settings panel on a paused film). Only a running picture may bring the
// audio back.
export function mayResumeAudioContext(o: { state: AudioContextState | string; armed: boolean; videoPaused: boolean }): boolean {
  return o.state === "suspended" && o.armed && !o.videoPaused;
}

// fillAudioBuffer copies a decoded PCM segment into a Web Audio buffer whose
// channel layout is already the mix's output order. Exported as a pure seam
// so the mapping (planar/interleaved, and the 7.1 fold) can be unit-tested
// without a real AudioContext.
export function fillAudioBuffer(
  target: AudioBuffer,
  d: PcmDecoded,
  mix: Array<Array<[number, number]>>,
): void {
  if ("planar" in d) {
    if (d.mixed) {
      // Already in output order with the taps applied: one native copy per
      // channel and nothing else, whatever the layout.
      for (let co = 0; co < d.planar.length; co++) target.copyToChannel(d.planar[co], co);
      return;
    }
    for (let co = 0; co < mix.length; co++) {
      const taps = mix[co];
      if (taps.length === 1 && taps[0][1] === 1) {
        // Pure permutation: a native copy, no JS loop over samples.
        target.copyToChannel(d.planar[taps[0][0]], co);
      } else {
        const out = target.getChannelData(co);
        for (let f = 0; f < d.frames; f++) {
          let s = 0;
          for (let t = 0; t < taps.length; t++) s += d.planar[taps[t][0]][f] * taps[t][1];
          out[f] = s;
        }
      }
    }
    return;
  }
  const ch = d.channels;
  for (let co = 0; co < mix.length; co++) {
    const out = target.getChannelData(co);
    const taps = mix[co];
    if (taps.length === 1) {
      const src = taps[0][0];
      for (let f = 0; f < d.frames; f++) out[f] = d.pcm[f * ch + src];
    } else {
      for (let f = 0; f < d.frames; f++) {
        let s = 0;
        for (let t = 0; t < taps.length; t++) s += d.pcm[f * ch + taps[t][0]] * taps[t][1];
        out[f] = s;
      }
    }
  }
}

export interface WasmAudioEngineOptions {
  // resolveUrl rewrites a URL with the freshest auth token before every fetch,
  // mirroring hls.js's xhrSetup so a long buffer-ahead can't 401 mid-film.
  resolveUrl?: (url: string) => string;
  // onFatal is called when the engine cannot continue (wasm load failed, the
  // playlist is unreadable, decode errors on the very first segments). The host
  // should tear the engine down and fall back to the AAC transcode path.
  onFatal?: (reason: string) => void;
  // aacPlaylistUrl enables the bi-source relay: the AAC rendition of the SAME
  // track (master's #EVEY-AUDIO-AAC-URI), decoded NATIVELY (decodeAudioData)
  // for sound within seconds while the wasm decoder boots; the engine then
  // hands over to the AC-3 playlist at a segment boundary. Either leg failing
  // latches the other; only both failing is fatal.
  aacPlaylistUrl?: string;
  // onModeChange reports relay leg transitions ("aac", "wasm", "aac-latched")
  // - telemetry only, no behaviour.
  onModeChange?: (mode: string) => void;
  // onDiagnostic reports WHY the relay took a decision the mode alone cannot
  // explain: a refused hand-over, a starvation fallback. Diagnosing the
  // 2026-07-30 silent-cast incident took hours of log archaeology precisely
  // because "aac-latched" does not say what latched it. Telemetry only.
  onDiagnostic?: (detail: string) => void;
  // onFirstAudio fires ONCE, when the first segment lands in the decode cache
  // - i.e. the engine can produce sound the moment the picture advances. The
  // cast receiver holds the video start on it so picture and sound begin
  // together, like the muxed master did.
  onFirstAudio?: () => void;
  // onStallGate fires each time the stall gate actually CUTS the sound: the
  // picture stopped advancing while audio was anchored or scheduled, so the
  // engine suspended the context and dropped the schedule. Called with the
  // running count. A server that serves every segment late trips this at each
  // boundary, and from the viewer's seat that is "no sound" - the 2026-09-02
  // beta report could only be inferred, the gate left no trace. Telemetry only.
  onStallGate?: (cuts: number) => void;
  // onSilentAudio fires ONCE per engine when the wasm leg has decoded ten
  // seconds of consecutive pure digital silence: a decoder that loses the
  // track without failing (ac3go's silent E-AC-3 downmix, 2026-09-27) looks
  // healthy to every other guard here. Telemetry only.
  onSilentAudio?: (detail: string) => void;
  leadCompensationSec?: number;
  // userOffsetSec is the viewer's own A/V correction from the player menu
  // (positive = audio later, the server's audioOffsetMs convention), applied
  // ON TOP of the measured lead. It exists because the lead only sees what the
  // browser reports: a Bluetooth or HDMI/ARC sink that buffers 0.3-1 s past
  // outputLatency is invisible to the estimator (prod 2026-09-13, -900 ms by
  // ear with lead=-35ms measured), and the menu used to reach only the native
  // AAC leg. Live-adjustable through setUserOffsetSec.
  userOffsetSec?: number;
  // onAuthError is called when a segment fetch returns 401/403 - most often
  // the sliding stream token expiring across a pause longer than its TTL
  // (hls.js's own buffer rides through it silently; this engine's shorter
  // look-ahead does not). Resolution means "a fresh token now lives wherever
  // resolveUrl reads it" - the pump loop's next retry (after its backoff)
  // picks it up. Optional: without it an expired token just backs off like
  // any other failure instead of healing. Deduped internally: several
  // segments failing at once trigger only one call.
  onAuthError?: () => Promise<string | void>;
  // Credentials for playlist/segment fetches. The web (same-origin) keeps
  // "include" (hls cookie). The Cast Web Receiver is CROSS-origin with the
  // media server: the CORS token-auth reflects the Origin WITHOUT
  // Allow-Credentials, so a credentialed request would be REJECTED by the
  // browser - the receiver passes "omit", the token in the URL is enough.
  credentials?: RequestCredentials;
}

// USER_OFFSET_MAX_SEC bounds the menu correction like the server does its
// audioOffsetMs (+/-30 s): a resync landing that far off is still a valid
// schedule, anything beyond is a bug, not a setting.
const USER_OFFSET_MAX_SEC = 30;

function clampUserOffset(sec: number): number {
  if (!Number.isFinite(sec)) return 0;
  return Math.min(USER_OFFSET_MAX_SEC, Math.max(-USER_OFFSET_MAX_SEC, sec));
}

export class WasmAudioEngine {
  private readonly video: HTMLVideoElement;
  private readonly playlistUrl: string;
  private readonly opts: WasmAudioEngineOptions;
  // Audio-lead compensation: static base (opts/default) superseded by the
  // MEASUREMENT once it's mature - requestVideoFrameCallback gives the frame
  // actually displayed, outputLatency the audio output; the gap between the
  // two is the pipeline's real latency (0.1s desktop, 0.5-0.6s on a cast TV).
  private readonly leadCompBase: number;
  private userOffsetSec: number;
  private readonly leadEstimator = new AVLatencyEstimator();
  private rvfcHandle: number | null = null;

  private worker: Worker | null = null;
  private ctx: AudioContext | null = null;
  private gain: GainNode | null = null;

  private segments: Segment[] = [];
  // Relay state. nativeState: "off" (classic wasm-only run), "active" (AAC leg
  // is the current source), "failed" (latched off). wasmState: "pending" while
  // the worker boots, "ready", "failed" (latched off - AAC keeps playing).
  private nativeState: "off" | "active" | "failed" = "off";
  private wasmState: "pending" | "ready" | "failed" = "pending";
  private aacCfg: AacConfig | null = null;
  private ac3Segments: Segment[] = [];
  // nativeSegments is the UNMERGED AAC timeline, kept for the whole session:
  // after the hand-over this.segments is a merged array, so the starvation
  // rescue would have nothing to hand the film back to.
  private nativeSegments: Segment[] = [];
  // owedSince: wall time at which the wasm leg started owing a segment with
  // nothing delivered since (null = owes nothing). The starvation watchdog
  // reads it on every tick; a successful wasm decode resets it. See owedSince
  // in relayGuard for why it is a debt clock and not a "last decode" clock.
  private owedSince: number | null = null;
  // Audio-output health (audioHealth.ts): when the context was first seen
  // suspended while the picture advanced, the last moment its clock moved,
  // how many heals were attempted and when the next one may run.
  private suspendedSince: number | null = null;
  private ctxClock: ClockSample | null = null;
  private heals = 0;
  private nextHealAt = 0;
  // wasmMemMB is the decoder's linear memory as reported by the worker. The
  // receiver's mem= beacon only sees the JS heap, which WebAssembly.Memory
  // never appears in - so a memory death used to be indistinguishable from a
  // flat, healthy heap (2026-07-30).
  private wasmMemMB = -1;
  private switched = false;
  // Timeline generation: bumped whenever this.segments is REPLACED (the
  // hand-over, a leg failure). In-flight decodes captured under an older
  // generation are discarded - their index points into a stale array.
  private timelineGen = 0;
  // Preroll cache: the last AAC frames of the most recently decoded native
  // segment. Decoding segment N with the tail of N-1 prepended (then skipping
  // those samples) gives the MDCT overlap its left context - without it every
  // 3s boundary carries an audible ~20ms attack glitch.
  private prerollCache: { idx: number; frames: Uint8Array[] } | null = null;
  // One-shot latch for opts.onFirstAudio.
  private firstAudioFired = false;

  private anchored = false;
  private anchorCtx = 0;
  private anchorMedia = 0;
  private anchorRate = 1;

  // Decode is DECOUPLED from scheduling: pump() decodes segments ahead into
  // decodedCache (even while the video is still buffering its first frames), so
  // the audio is ready the instant the picture starts and does not lag it.
  // scheduleWindow() then wires the already-decoded segments onto the clock.
  private decodedCache = new Map<number, DecodedSegment>();
  private inflight = new Set<number>();
  private scheduledIdx = new Set<number>();
  private activeSources = new Set<AudioBufferSourceNode>();
  // Stall gate: the last observed currentTime. The audio only plays while the
  // picture is genuinely ADVANCING; a frozen currentTime (startup buffering, a
  // mid-play stall) freezes the audio too, instead of letting it run - and loop
  // on re-sync - ahead of a stalled picture.
  private lastCt = -1;
  // Running count of stall-gate cuts, for onStallGate.
  private stallCuts = 0;
  private timer: ReturnType<typeof setInterval> | null = null;
  private pumping = false;
  private destroyed = false;
  private started = false;
  // Fallback tripwire: true once ANY segment has been scheduled. Until then, a
  // run of decode failures means the stream is undecodable (e.g. a codec variant
  // ac3go can't handle) and we must fall back - the guard cannot key on
  // segments[0] because a deep resume starts mid-file, never at segment 0.
  private everScheduled = false;
  private decodeFailStreak = 0;
  private silence = new SilenceRun();
  // Per-index backoff/attempt-cap state for pump()'s retry loop - see
  // segmentRetry.ts for the policy itself and why it exists.
  private readonly retryTracker = new SegmentRetryTracker();
  // Single-flight gate on opts.onAuthError: several segments can 401 within
  // the same pump pass (a whole in-flight window went stale together), and
  // without this each would fire its own refresh call.
  private authRefreshInFlight: Promise<unknown> | null = null;

  private decodeSeq = 0;
  private pendingDecodes = new Map<number, { resolve: (d: DecodedSegment) => void; reject: (e: Error) => void }>();

  private readonly boundHandlers: Array<[string, EventListener]> = [];
  private gestureUnlock: (() => void) | null = null;
  // Armed = allowed to attempt an AudioContext resume. Disarmed after a refused
  // (autoplay-blocked) resume so the tick can't spam the console; re-armed on a
  // successful resume or a user gesture. See resumeCtx().
  private ctxRetry = true;

  constructor(video: HTMLVideoElement, playlistUrl: string, opts: WasmAudioEngineOptions = {}) {
    this.video = video;
    this.playlistUrl = playlistUrl;
    this.opts = opts;
    this.leadCompBase = opts.leadCompensationSec ?? DEFAULT_LEAD_COMP_SEC;
    this.userOffsetSec = clampUserOffset(opts.userOffsetSec ?? 0);
  }

  private resolve(url: string): string {
    return this.opts.resolveUrl ? this.opts.resolveUrl(url) : url;
  }

  private fatal(reason: string): void {
    if (this.destroyed) return;
    console.warn("[WasmAudioEngine] fatal:", reason);
    this.opts.onFatal?.(reason);
  }

  // requestAuthRefresh asks the host for a fresh token, deduping concurrent
  // callers (see authRefreshInFlight). Fire-and-forget from the pump loop's
  // point of view: the token lands in whatever resolveUrl reads, and the
  // segment's own backoff (already recorded by the caller) is what schedules
  // the retry that picks it up - this method does not retry anything itself.
  private requestAuthRefresh(): void {
    if (!this.opts.onAuthError || this.authRefreshInFlight || this.destroyed) return;
    this.authRefreshInFlight = Promise.resolve(this.opts.onAuthError())
      .catch((err) => {
        console.warn("[WasmAudioEngine] auth refresh failed:", err);
      })
      .finally(() => {
        this.authRefreshInFlight = null;
      });
  }

  async start(): Promise<void> {
    if (this.started || this.destroyed) return;
    this.started = true;
    try {
      if (this.opts.aacPlaylistUrl && (await this.startNativeLeg(this.opts.aacPlaylistUrl))) {
        // Relay: sound rides the AAC leg NOW; the wasm leg boots in the
        // background and takes over at a segment boundary once ready.
        this.nativeState = "active";
        void this.bootWasmLeg();
      } else {
        // Classic wasm-only run (or the AAC leg refused): everything must be
        // up before scheduling starts.
        await this.initWorker();
        if (this.destroyed) return;
        const { segs } = await this.loadPlaylist(this.playlistUrl);
        this.segments = segs;
        this.wasmState = "ready";
      }
      if (this.destroyed) return;
      this.initAudio();
      this.attachVideoEvents();
      this.startScheduling();
      this.opts.onModeChange?.(this.mode());
    } catch (err) {
      this.fatal(err instanceof Error ? err.message : String(err));
    }
  }

  // mode reports the live source for telemetry: "aac" (native leg playing,
  // wasm still booting), "wasm" (wasm leg playing - classic or post-handover),
  // "aac-latched" (wasm failed, native carries the whole film).
  // decoderMemoryMB is the wasm module's LINEAR memory in MB, or -1 before the
  // first decode. Deliberately separate from the receiver's mem= (the JS heap):
  // a WebAssembly.Memory that grows never shows up there, so a flat heap beacon
  // could not distinguish a healthy cast from one about to be memory-killed.
  decoderMemoryMB(): number {
    return this.wasmMemMB;
  }

  mode(): string {
    if (this.nativeState === "active") return this.wasmState === "failed" ? "aac-latched" : this.switched ? "wasm" : "aac";
    return "wasm";
  }

  // --- playlist -----------------------------------------------------------

  private async loadPlaylist(url: string): Promise<{ segs: Segment[]; mapUri: string | null }> {
    const resp = await fetch(this.resolve(url), { credentials: this.opts.credentials ?? "include" });
    if (!resp.ok) throw new Error(`audio playlist ${resp.status}`);
    const text = await resp.text();
    const base = new URL(url, window.location.origin);
    const segs: Segment[] = [];
    let mapUri: string | null = null;
    let start = 0;
    let pendingDur = 0;
    for (const raw of text.split("\n")) {
      const line = raw.trim();
      if (!line) continue;
      if (line.startsWith("#EXTINF:")) {
        pendingDur = parseFloat(line.slice("#EXTINF:".length)) || 0;
        continue;
      }
      if (line.startsWith("#EXT-X-MAP:")) {
        // The AC-3 demuxer never needs the init (samples are self-describing),
        // but the native AAC leg does: its AudioSpecificConfig lives there.
        const m = line.match(/URI="([^"]+)"/);
        if (m) mapUri = new URL(m[1], base).toString();
        continue;
      }
      if (line.startsWith("#")) continue;
      const url2 = new URL(line, base).toString();
      segs.push({ url: url2, start, dur: pendingDur });
      start += pendingDur;
      pendingDur = 0;
    }
    if (segs.length === 0) throw new Error("audio playlist has no segments");
    return { segs, mapUri };
  }

  // --- native (AAC) leg ---------------------------------------------------

  // startNativeLeg loads the AAC rendition playlist + init and prepares the
  // native timeline. Returns false (never throws) when anything about the
  // shape is off - the caller then falls back to the classic wasm-only start.
  private async startNativeLeg(aacUrl: string): Promise<boolean> {
    try {
      const { segs, mapUri } = await this.loadPlaylist(aacUrl);
      if (!mapUri) return false;
      const initResp = await fetch(this.resolve(mapUri), { credentials: this.opts.credentials ?? "include" });
      if (!initResp.ok) return false;
      const cfg = parseAacConfig(new Uint8Array(await initResp.arrayBuffer()));
      if (!cfg) return false;
      this.aacCfg = cfg;
      this.segments = segs.map((s) => ({ ...s, native: true }));
      this.nativeSegments = this.segments;
      return true;
    } catch {
      return false;
    }
  }

  // bootWasmLeg brings the wasm worker + AC-3 playlist up WHILE the native leg
  // plays, then plans the hand-over. Failure latches the native leg as the
  // sole source - strictly better than dying.
  private async bootWasmLeg(): Promise<void> {
    try {
      await this.initWorker();
      if (this.destroyed) return;
      const { segs } = await this.loadPlaylist(this.playlistUrl);
      if (this.destroyed) return;
      this.ac3Segments = segs;
      // "Ready" is not "fast enough". Probe ONE real segment before handing the
      // film over: on a weak box the decoder can be up and still be unable to
      // sustain real time, which produces a silent picture and, a couple of
      // minutes later, a receiver killed by the box (prod, 2026-07-30).
      // Only relevant when there IS an AAC leg to stay on; the classic
      // wasm-only path has no alternative and must not be blocked.
      if (this.nativeState === "active" && !(await this.probeWasmLeg())) return;
      this.wasmState = "ready";
      this.planHandover();
    } catch (err) {
      console.warn("[WasmAudioEngine] wasm leg failed, AAC latched:", err);
      this.wasmState = "failed";
      if (this.nativeState === "failed") {
        this.fatal("both relay legs failed");
        return;
      }
      this.opts.onModeChange?.(this.mode());
    }
  }

  // failNativeLeg is the native counterpart of bootWasmLeg's catch: decode is
  // broken (device without AAC decodeAudioData, hostile bytes). If the wasm
  // leg is up, adopt its full timeline now; if it is still booting, planHandover
  // will do it on arrival; if it failed too, nothing can play.
  private failNativeLeg(reason: string): void {
    if (this.nativeState !== "active") return;
    this.nativeState = "failed";
    console.warn("[WasmAudioEngine] native AAC leg failed:", reason);
    if (this.wasmState === "ready") {
      this.adoptTimeline(this.ac3Segments);
      this.switched = true;
      this.opts.onModeChange?.(this.mode());
    } else if (this.wasmState === "failed") {
      this.fatal("both relay legs failed");
    }
  }

  // probeWasmLeg decodes ONE real AC-3 segment and asks relayGuard whether this
  // device can carry the wasm leg. Returns false when the hand-over must NOT
  // happen - the AAC leg then finishes the film (mode "aac-latched").
  //
  // The probe costs one segment decode, which the hand-over would have paid for
  // anyway; the difference is that here the result is judged instead of assumed.
  private async probeWasmLeg(): Promise<boolean> {
    const seg = this.ac3Segments[this.ac3IndexAt(this.video.currentTime)];
    if (!seg) return true; // nothing to probe: let the normal path decide
    let verdict: HandoverVerdict;
    try {
      const t0 = Date.now();
      const decoded = await this.decodeSegment(-1, seg);
      const decodeMs = Date.now() - t0;
      if (this.destroyed) return false;
      verdict = shouldHandover({
        decodeMs,
        segmentDurSec: seg.dur,
        channels: decoded.kind === "pcm" ? decoded.channels : (decoded.audioBuffer?.numberOfChannels ?? 0),
      });
    } catch (err) {
      // A probe that cannot even decode is the clearest possible "no".
      verdict = { ok: false, reason: "probe: " + (err instanceof Error ? err.message : String(err)) };
    }
    if (verdict.ok) return true;
    // Refused BEFORE any hand-over: nothing is committed yet, so the right move
    // is not to keep this engine limping on its AAC leg - it is to leave the
    // wasm topology ENTIRELY. The host's fallback reloads the plain muxed
    // master: ordinary audio+video, no Web Audio, no video-only trick, no
    // worker. On a box that refuses the decode, that also removes every layer
    // this engine adds, which is the safest thing to hand a fragile device.
    // (Starvation mid-film is the opposite case - see revertToNativeLeg: there
    // a reload would interrupt a film that is playing, so we latch in place.)
    this.opts.onDiagnostic?.(verdict.reason ?? "probe refused");
    this.fatal(verdict.reason ?? "probe refused");
    return false;
  }

  // latchNative pins the AAC leg as the sole source for the rest of the film and
  // reports why. Used both by the pre-hand-over probe and by the starvation
  // watchdog; wasmState="failed" is what makes mode() read "aac-latched" and
  // stops any further hand-over attempt.
  private latchNative(reason: string): void {
    if (this.wasmState === "failed") return;
    this.wasmState = "failed";
    this.switched = false;
    console.warn("[WasmAudioEngine] AAC latched:", reason);
    this.opts.onDiagnostic?.(reason);
    this.opts.onModeChange?.(this.mode());
  }

  // wasmDeliveryPending: is there a wasm segment in the look-ahead window the
  // leg has neither decoded nor scheduled? That, and not the age of the last
  // decode, is what "the leg is not delivering" means - the starvation guard
  // reads it so a window the cache already covers cannot be mistaken for a dead
  // decoder (see isStarved). Decoded-and-cached counts as delivered: the bytes
  // exist, scheduling them is the anchor's business, not the decoder's.
  private wasmDeliveryPending(ct: number): boolean {
    for (let idx = this.segmentIndexAt(ct); idx < this.segments.length; idx++) {
      const seg = this.segments[idx];
      if (seg.start > ct + BUFFER_AHEAD_SEC) break;
      if (seg.native) continue; // the AAC leg's, never evidence about this one
      if (seg.start + seg.dur <= ct - 0.05) continue; // wholly past
      if (!this.decodedCache.has(idx) && !this.scheduledIdx.has(idx)) return true;
    }
    return false;
  }

  // ac3IndexAt is segmentIndexAt against the AC-3 timeline, which is not
  // this.segments until the hand-over has happened.
  private ac3IndexAt(mediaTime: number): number {
    for (let i = 0; i < this.ac3Segments.length; i++) {
      const s = this.ac3Segments[i];
      if (mediaTime < s.start + s.dur) return i;
    }
    return Math.max(0, this.ac3Segments.length - 1);
  }

  // revertToNativeLeg is the starvation rescue: the wasm leg stopped delivering
  // while the picture kept running, so hand the REST of the film back to the AAC
  // leg. Symmetric to the hand-over - planRelayHandover is direction-agnostic,
  // it keeps the live prefix of the current timeline and splices the other one
  // in at the boundary - so scheduled audio is never un-scheduled.
  private revertToNativeLeg(reason: string): void {
    if (this.destroyed || !this.switched || this.nativeState !== "active") return;
    const plan = planRelayHandover(this.segments, this.nativeSegments, this.scheduledIdx, this.video.currentTime);
    if (plan) this.adoptTimeline(plan.segments, plan.keepPrefix);
    this.latchNative(reason);
    void this.pump();
  }

  // planHandover applies planRelayHandover's pure computation to the live
  // engine state once the wasm leg is up.
  private planHandover(): void {
    if (this.destroyed || this.switched) return;
    if (this.nativeState !== "active") {
      // Native leg already failed: the AC-3 timeline is the only source.
      this.adoptTimeline(this.ac3Segments);
      this.switched = true;
      this.opts.onModeChange?.(this.mode());
      return;
    }
    const plan = planRelayHandover(this.segments, this.ac3Segments, this.scheduledIdx, this.video.currentTime);
    if (!plan) return; // film ends before the boundary: the AAC leg finishes it
    this.adoptTimeline(plan.segments, plan.keepPrefix);
    this.switched = true;
    this.opts.onModeChange?.(this.mode());
  }

  // adoptTimeline swaps this.segments and invalidates everything that indexes
  // into the old array from keepPrefix onward. The generation bump makes any
  // in-flight decode completion drop its result instead of caching it under an
  // index that now names a different segment.
  private adoptTimeline(segs: Segment[], keepPrefix = 0): void {
    this.segments = segs;
    this.timelineGen++;
    // planRelayHandover puts the boundary at/after the END of every scheduled
    // segment, so nothing scheduled should ever sit past keepPrefix. If that
    // invariant is ever broken, the already-STARTED sources beyond the prefix
    // would keep playing UNDER the replacement segments (double audio) - so
    // detect it and take the conservative reset: stop everything, let the next
    // tick re-anchor and re-schedule from the (kept) cache. A brief hiccup
    // beats seconds of doubled sound.
    let staleScheduled = false;
    // Deleting during iteration is safe on JS Map/Set iterators.
    for (const idx of this.decodedCache.keys()) {
      if (idx >= keepPrefix) this.decodedCache.delete(idx);
    }
    for (const idx of this.scheduledIdx) {
      if (idx >= keepPrefix) {
        this.scheduledIdx.delete(idx);
        staleScheduled = true;
      }
    }
    if (staleScheduled) {
      this.clearScheduled();
      this.anchored = false;
    }
    this.inflight.clear();
    this.prerollCache = null;
    // Old indices name different segments in the new timeline - their backoff
    // history no longer applies.
    this.retryTracker.clearAll();
  }

  // --- worker -------------------------------------------------------------

  private initWorker(): Promise<void> {
    return new Promise((resolve, reject) => {
      const worker = new Worker(WORKER_URL);
      this.worker = worker;
      worker.onmessage = (e: MessageEvent) => {
        const msg = e.data;
        if (msg.type === "ready") {
          resolve();
        } else if (msg.type === "error") {
          reject(new Error("wasm init: " + msg.error));
        } else if (msg.type === "decoded") {
          if (typeof msg.wasmMemMB === "number") this.wasmMemMB = msg.wasmMemMB;
          const p = this.pendingDecodes.get(msg.id);
          if (p) {
            this.pendingDecodes.delete(msg.id);
            // msg.planar: fresh worker. msg.pcm: a cached old ac3Worker.js
            // still running the interleaved protocol - keep serving it.
            if (msg.planar) {
              p.resolve({
                kind: "pcm",
                channels: msg.channels,
                sampleRate: msg.sampleRate,
                frames: msg.frames,
                planar: (msg.planar as ArrayBuffer[]).map((b) => new Float32Array(b)),
                mixed: msg.mixed === true,
              });
            } else {
              p.resolve({
                kind: "pcm",
                channels: msg.channels,
                sampleRate: msg.sampleRate,
                frames: msg.frames,
                pcm: new Float32Array(msg.pcm),
              });
            }
          }
        } else if (msg.type === "decodeError") {
          const p = this.pendingDecodes.get(msg.id);
          if (p) {
            this.pendingDecodes.delete(msg.id);
            p.reject(new Error(msg.error));
          }
        }
      };
      worker.onerror = (e) => reject(new Error("worker error: " + e.message));
      worker.postMessage({ type: "init", wasmUrl: WASM_URL, wasmExecUrl: WASM_EXEC_URL, mixes: workerMixTable() });
    });
  }

  private decodeSegment(idx: number, seg: Segment): Promise<DecodedSegment> {
    if (seg.native) return this.decodeNativeSegment(idx, seg);
    return new Promise<DecodedSegment>((resolve, reject) => {
      const worker = this.worker;
      if (!worker) {
        reject(new Error("no worker"));
        return;
      }
      fetch(this.resolve(seg.url), { credentials: this.opts.credentials ?? "include" })
        .then((r) => {
          if (!r.ok) throw new SegmentHttpError(r.status);
          return r.arrayBuffer();
        })
        .then((buf) => {
          const id = ++this.decodeSeq;
          this.pendingDecodes.set(id, { resolve, reject });
          // Downmix decision is deliberately absent: we decode the full layout
          // and let Web Audio downmix to the output device, so a 5.1 output gets
          // discrete channels while a stereo one gets a proper fold-down.
          worker.postMessage({ type: "decode", id, bytes: buf }, [buf]);
        })
        .catch(reject);
    });
  }

  // decodeNativeSegment: fetch the fMP4 AAC segment, demux its frames, wrap in
  // ADTS, decodeAudioData. When the previous segment's tail frames are at hand
  // (sequential pump - the common case) they are prepended and their samples
  // skipped, giving the first real frame its MDCT left context.
  private async decodeNativeSegment(idx: number, seg: Segment): Promise<DecodedSegment> {
    const ctx = this.ctx;
    const cfg = this.aacCfg;
    if (!ctx || !cfg) throw new Error("native leg not initialised");
    const r = await fetch(this.resolve(seg.url), { credentials: this.opts.credentials ?? "include" });
    if (!r.ok) throw new SegmentHttpError(r.status);
    const bytes = new Uint8Array(await r.arrayBuffer());
    const frames = extractAacFrames(bytes);
    if (frames.length === 0) throw new Error("no AAC frames in segment");
    const pre = this.prerollCache && this.prerollCache.idx === idx - 1 ? this.prerollCache.frames : [];
    const adts = adtsFromFrames([...pre, ...frames], cfg);
    // Copy into an exact-size ArrayBuffer: decodeAudioData detaches its input.
    const adtsCopy = new ArrayBuffer(adts.byteLength);
    new Uint8Array(adtsCopy).set(adts);
    const audioBuffer = await ctx.decodeAudioData(adtsCopy);
    this.prerollCache = { idx, frames: frames.slice(-2).map((f) => f.slice()) };
    return {
      kind: "buffer",
      audioBuffer,
      offsetSec: (pre.length * AAC_SAMPLES_PER_FRAME) / audioBuffer.sampleRate,
    };
  }

  // --- audio graph --------------------------------------------------------

  private initAudio(): void {
    const AC: typeof AudioContext =
      window.AudioContext ??
      (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
    const ctx = new AC();
    const gain = ctx.createGain();
    // Soft limiter identical to useVolumeBoost: transparent below -3 dBFS, tames
    // the overshoot when the boost pushes gain past 1.
    const comp = ctx.createDynamicsCompressor();
    comp.threshold.value = -3;
    comp.knee.value = 6;
    comp.ratio.value = 12;
    comp.attack.value = 0.003;
    comp.release.value = 0.25;
    gain.connect(comp);
    comp.connect(ctx.destination);
    this.ctx = ctx;
    this.gain = gain;
    this.applyGain();

    // DEV-only introspection for the Playwright sync harness (stripped from prod
    // builds by the import.meta.env.DEV dead-code elimination).
    if (import.meta.env.DEV) {
      (window as unknown as { __wasmAudio?: WasmAudioEngine }).__wasmAudio = this;
    }

    // An AudioContext starts suspended until a user gesture. The video may
    // autoplay muted without one, so arm a one-shot gesture unlock that resumes
    // the context; also resume whenever the video plays.
    const unlock = () => {
      this.ctxRetry = true; // a real gesture: re-arm and resume within it
      this.resumeCtx();
    };
    const events = ["pointerdown", "keydown", "touchstart"];
    events.forEach((ev) => window.addEventListener(ev, unlock, { passive: true }));
    this.gestureUnlock = () => events.forEach((ev) => window.removeEventListener(ev, unlock));
    this.resumeCtx();
  }

  // resumeCtx attempts to resume a suspended context, then DISARMS itself until
  // the resume actually lands or a gesture re-arms it. Autoplay-blocked resume()
  // calls each log "AudioContext couldn't start automatically", and the tick
  // would call this constantly - so we try ONCE, and only retry after it
  // succeeds (a previously-unlocked context we suspended for a stall/pause) or a
  // user gesture. When autoplay IS allowed, or the page already has activation
  // (the click that opened the player), the one attempt succeeds immediately.
  private resumeCtx(): void {
    const ctx = this.ctx;
    if (!ctx) return;
    if (!mayResumeAudioContext({ state: ctx.state, armed: this.ctxRetry, videoPaused: this.video.paused })) return;
    this.ctxRetry = false;
    ctx
      .resume()
      .then(() => {
        if (ctx.state === "running") this.ctxRetry = true; // re-arm for next cycle
      })
      .catch(() => {
        /* refused for lack of a gesture - stay disarmed until unlock() */
      });
  }

  // Volume/mute mirror the <video> element: the wasm-audio path serves a
  // video-only stream (no element audio), so the native volume UI - which drives
  // video.volume / video.muted - carries the user's intent, and this engine's
  // gain simply follows it via the 'volumechange' event. The 0-100% range and
  // mute are fully honoured; the >100% boost (a separate graph on the muted
  // element) is not amplified here - a documented first-version limitation.
  private applyGain(): void {
    if (!this.gain || !this.ctx) return;
    const target = this.video.muted ? 0 : this.video.volume;
    this.gain.gain.setTargetAtTime(target, this.ctx.currentTime, 0.02);
  }

  // --- video events -------------------------------------------------------

  private on(target: EventTarget, ev: string, fn: EventListener): void {
    target.addEventListener(ev, fn);
    this.boundHandlers.push([ev, fn]);
  }

  private attachVideoEvents(): void {
    const v = this.video;
    this.on(v, "seeking", () => {
      this.clearScheduled();
      this.anchored = false;
    });
    this.on(v, "seeked", () => {
      // Re-baseline the stall gate at the new position and warm the cache there;
      // the tick re-anchors once the picture is advancing again.
      this.lastCt = v.currentTime;
      void this.pump();
    });
    // Pause: suspend immediately (the tick keeps it frozen and, being paused,
    // preserves the anchor + sources for a seamless resume).
    this.on(v, "pause", () => {
      void this.ctx?.suspend();
    });
    // Resume: unfreeze immediately; the tick re-anchors/re-schedules as needed.
    this.on(v, "playing", () => {
      if (!v.paused) this.resumeCtx();
    });
    this.on(v, "ratechange", () => {
      // A non-1x playback rate changes the media/wall-clock slope; re-anchor so
      // the mapping tracks it (a full trick-play audio pitch shift is out of
      // scope - this keeps 1x correct and avoids runaway drift off 1x).
      this.resync();
    });
    // Mirror the element's volume/mute (the native UI's target).
    this.on(v, "volumechange", () => this.applyGain());
  }

  // --- scheduling ---------------------------------------------------------

  // ctxForMedia maps a media timestamp to the AudioContext clock. At a non-1x
  // playback rate the media advances rate× faster than wall-clock (the context
  // clock), so the media delta is divided by the rate; the buffer sources are
  // sped up to match (scheduleDecoded), pitch-shifting like ordinary fast play.
  private ctxForMedia(mediaTime: number): number {
    return this.anchorCtx + (mediaTime - this.anchorMedia) / this.anchorRate;
  }

  private reanchor(ct: number): void {
    if (!this.ctx) return;
    this.anchorCtx = this.ctx.currentTime;
    this.anchorMedia = ct;
    this.anchorRate = this.video.playbackRate || 1;
    this.anchored = true;
  }

  // resync drops the current schedule and unanchors; the next advancing tick
  // re-anchors at the fresh position and re-schedules from the (pre-decoded)
  // cache. Used on seek / large drift.
  private resync(): void {
    if (this.destroyed) return;
    this.clearScheduled();
    this.anchored = false;
  }

  private segmentIndexAt(mediaTime: number): number {
    // Last segment whose start is <= mediaTime.
    let idx = 0;
    for (let i = 0; i < this.segments.length; i++) {
      if (this.segments[i].start <= mediaTime) idx = i;
      else break;
    }
    return idx;
  }

  private clearScheduled(): void {
    for (const s of this.activeSources) {
      try {
        s.onended = null;
        s.stop();
      } catch {
        /* already stopped */
      }
    }
    this.activeSources.clear();
    this.scheduledIdx.clear();
  }

  private get leadComp(): number {
    return this.leadEstimator.estimate ?? this.leadCompBase;
  }

  // setUserOffsetSec applies a new menu correction immediately: the schedule is
  // dropped and rebuilt from the decoded cache at the new offset (a resync,
  // like a seek) - no reload of the video leg, which the AAC path needs.
  setUserOffsetSec(sec: number): void {
    const v = clampUserOffset(sec);
    if (v === this.userOffsetSec || this.destroyed) return;
    this.userOffsetSec = v;
    if (this.anchored || this.scheduledIdx.size > 0) this.resync();
  }

  userOffset(): number {
    return this.userOffsetSec;
  }

  // A/V measurement loop: on every PRESENTED frame, sample
  // (video display delay) - (audio output latency). Best-effort:
  // without requestVideoFrameCallback, only the static base is used.
  private startLatencyMeasurement(): void {
    const v = this.video as HTMLVideoElement & {
      requestVideoFrameCallback?: (cb: (now: number, meta: { mediaTime: number; expectedDisplayTime: number }) => void) => number;
      cancelVideoFrameCallback?: (handle: number) => void;
    };
    if (typeof v.requestVideoFrameCallback !== "function") return;
    const loop = (now: number, meta: { mediaTime: number; expectedDisplayTime: number }): void => {
      if (this.destroyed) return;
      if (!this.video.paused) {
        const videoLagSec =
          (this.video.currentTime - meta.mediaTime) +
          Math.max(0, (meta.expectedDisplayTime - now) / 1000);
        const audioLatSec =
          (this.ctx as (AudioContext & { outputLatency?: number }) | null)?.outputLatency ??
          this.ctx?.baseLatency ??
          0;
        this.leadEstimator.update(videoLagSec - audioLatSec);
      }
      this.rvfcHandle = v.requestVideoFrameCallback!(loop);
    };
    this.rvfcHandle = v.requestVideoFrameCallback(loop);
  }

  private startScheduling(): void {
    this.startLatencyMeasurement();
    this.lastCt = this.video.currentTime;
    void this.pump(); // start decoding immediately, don't wait a tick
    this.timer = setInterval(() => this.tick(), SCHEDULE_TICK_MS);
  }

  private tick(): void {
    if (this.destroyed || !this.ctx) return;
    const ct = this.video.currentTime;
    // Decode ahead ALWAYS (even paused/stalled) so start/resume is instant.
    void this.pump();

    // "Advancing" = the picture's clock actually moved since last tick. A frozen
    // currentTime (buffering, stall) or a paused element is NOT advancing.
    const advancing = !this.video.paused && ct > this.lastCt + 1e-3;
    this.lastCt = ct;

    // Starvation rescue: the picture is running but the wasm leg has owed a
    // segment for a long while with nothing delivered. On a box that cannot
    // sustain the decode this is the difference between a film that finishes
    // in AAC and a silent picture the receiver dies on (prod, 2026-07-30).
    // Checked before the pause/stall returns below; owedSince charges nothing
    // to a paused or buffering picture (the 2026-09-03 "stays in buffering"
    // sessions latched the film to AAC on their first frame for that reason).
    const now = Date.now();
    this.owedSince = this.switched ? owedSince(this.owedSince, this.wasmDeliveryPending(ct), advancing, now) : null;
    if (isStarved({
      switched: this.switched,
      playing: advancing,
      owedForMs: this.owedSince == null ? 0 : now - this.owedSince,
    })) {
      this.revertToNativeLeg("starvation: no wasm audio decoded while the picture ran");
    }

    if (this.video.paused || !advancing) {
      // A picture that is not running makes no claim on the output: a
      // suspended context or a frozen clock is the expected state here.
      this.suspendedSince = null;
      this.ctxClock = null;
    }
    if (this.video.paused) {
      // Pause: freeze the audio clock seamlessly and KEEP the anchor + scheduled
      // sources, so resume continues exactly where it stopped (both clocks froze
      // together, so the mapping still holds).
      if (this.ctx.state === "running") void this.ctx.suspend();
      return;
    }
    if (!advancing) {
      // Playing but the picture is STALLED (buffering, esp. a cold transcode
      // start): freeze the audio AND drop the schedule/anchor. Recovery may jump
      // currentTime, and letting the audio run - and loop on re-sync - ahead of a
      // frozen frame is exactly the startup "audio loops while waiting" glitch.
      if (this.ctx.state === "running") void this.ctx.suspend();
      if (this.anchored || this.activeSources.size > 0) {
        this.anchored = false;
        this.clearScheduled();
        this.stallCuts++;
        this.opts.onStallGate?.(this.stallCuts);
      }
      return;
    }

    this.resumeCtx();
    if (this.ctx.state === "suspended") {
      // Not unlocked yet - wait for a gesture. Unless the picture has been
      // running against a suspended context for a while: then the one
      // resume() resumeCtx allowed itself never landed, and waiting for a
      // gesture is exactly the silent picture (2026-09-21). Re-arm and try
      // again, with a growing backoff.
      this.suspendedSince ??= now;
      if (stuckFor(this.suspendedSince, now) > AUDIO_STUCK_MS && now >= this.nextHealAt) {
        this.heal("audio context stuck suspended while the picture ran; retrying resume", now);
        this.ctxRetry = true;
        this.resumeCtx();
      }
      return;
    }
    this.suspendedSince = null;
    // A running context whose clock stopped is an output device that died
    // under it: everything scheduled from here on waits for a time that never
    // comes. Cycling the context re-opens the device (a seek does the same by
    // accident, which is how the 2026-09-21 viewer got the sound back).
    const clock = audioClockFrozen(this.ctxClock, now, this.ctx.currentTime);
    this.ctxClock = clock.sample;
    if (clock.frozen && now >= this.nextHealAt) {
      this.heal("audio clock frozen while the picture ran; cycling the context", now);
      const ctx = this.ctx;
      this.clearScheduled();
      this.anchored = false;
      this.ctxClock = null;
      void ctx.suspend().then(() => ctx.resume()).catch(() => { /* the next tick judges the result */ });
      return;
    }
    if (!this.anchored) {
      this.reanchor(ct);
    } else {
      // Drift watchdog: only a seek/stall exceeds the threshold; natural drift is
      // negligible and left alone (a re-anchor is audible).
      const audioMedia = this.anchorMedia + (this.ctx.currentTime - this.anchorCtx) * this.anchorRate;
      if (Math.abs(ct - audioMedia) > RESYNC_THRESHOLD_SEC) {
        this.resync();
        return;
      }
    }
    this.scheduleWindow(ct);
  }

  // pump keeps the look-ahead window decoded into decodedCache. It re-reads the
  // LIVE playhead on every iteration and always decodes the first missing segment
  // FROM THE CURRENT position - so a resume or seek that moves the playhead (even
  // one that lands after the engine already started pre-decoding from 0) switches
  // the decode to the new position after at most one in-flight segment, instead
  // of grinding through a stale window first (the deep-resume "no sound for
  // several seconds" bug). Runs regardless of anchor/stall so the buffer is warm.
  private async pump(): Promise<void> {
    // The worker gate only applies to wasm segments: the native leg decodes
    // through the AudioContext and must run while the worker still boots.
    if (this.pumping || this.destroyed) return;
    this.pumping = true;
    try {
      while (!this.destroyed) {
        const ct = this.video.currentTime;
        // Evict what the playhead can no longer reach cheaply: played out, or
        // left far ahead by a backward seek (memory bound, both directions).
        for (const idx of this.decodedCache.keys()) {
          const s = this.segments[idx];
          if (!s) continue;
          if (s.start + s.dur < ct - 4 || s.start > ct + BUFFER_AHEAD_SEC + EVICT_AHEAD_MARGIN_SEC) {
            this.decodedCache.delete(idx);
          }
        }
        // Paused: fetch nothing new once a first segment has ever scheduled -
        // a long pause (stream token TTL, viewer stepping away) must not keep
        // hammering fetches nothing will play soon (the 2026-09-08 401 storm:
        // pump() decoded ahead through an entire pause on an expired token).
        // A cold start still needs to fill in while paused (autoplay blocked,
        // player opened paused), so the very first segment is exempt.
        if (this.video.paused && this.everScheduled) break;
        // First not-yet-decoded segment in the window, measured from the LIVE ct.
        let target = -1;
        const now = Date.now();
        for (let idx = this.segmentIndexAt(ct); idx < this.segments.length; idx++) {
          const seg = this.segments[idx];
          if (seg.start > ct + BUFFER_AHEAD_SEC) break;
          if (seg.start + seg.dur <= ct - 0.05) continue; // wholly past
          if (this.decodedCache.has(idx) || this.inflight.has(idx)) continue;
          if (!seg.native && !this.worker) continue; // wasm segment, worker not up yet
          if (!this.retryTracker.canAttempt(idx, now)) continue; // backing off / exhausted
          target = idx;
          break;
        }
        if (target < 0) break; // window fully decoded (or every candidate is backing off)
        const seg = this.segments[target];
        const gen = this.timelineGen;
        this.inflight.add(target);
        try {
          const d = await this.decodeSegment(target, seg);
          if (this.destroyed) return;
          if (gen !== this.timelineGen) continue; // timeline swapped mid-decode: stale index
          this.decodedCache.set(target, d);
          this.decodeFailStreak = 0;
          this.retryTracker.clear(target);
          // Only wasm decodes settle the starvation debt: the native leg
          // delivering is precisely NOT evidence that the wasm leg is alive.
          if (!seg.native) this.owedSince = null;
          if (d.kind === "pcm" && this.silence.record(target, seg.dur, pcmIsSilent(d))) {
            this.opts.onSilentAudio?.(
              `run=${this.silence.currentRunSec.toFixed(1)}s silent=${this.silence.silentSec.toFixed(1)}s segs=${this.silence.segments} ch=${d.channels} rate=${d.sampleRate}`,
            );
          }
          if (!this.firstAudioFired) {
            // The pump decodes from the live playhead, so the FIRST cached
            // segment is the one the start needs.
            this.firstAudioFired = true;
            this.opts.onFirstAudio?.();
          }
        } catch (err) {
          if (gen !== this.timelineGen) continue;
          // Backoff/attempt-cap for THIS index, independent of everScheduled:
          // a 401/403 asks for a fresh token (deduped, at most once per
          // failure streak) in addition to backing off - the refresh does not
          // bypass the wait, in case the failure wasn't actually auth-related.
          if (this.retryTracker.recordFailure(target, Date.now(), isAuthStatus(err))) {
            this.requestAuthRefresh();
          }
          // A single bad segment once playback is going must not kill it. But if
          // NOTHING has ever scheduled and decodes keep failing, this source is
          // undecodable - fail its leg (relay) or the engine (classic).
          if (!this.everScheduled) {
            this.decodeFailStreak++;
            if (this.decodeFailStreak >= 3) {
              if (seg.native && this.wasmState !== "failed") {
                // Device without usable AAC decode: hand everything to wasm.
                this.decodeFailStreak = 0;
                this.failNativeLeg("decode: " + (err instanceof Error ? err.message : String(err)));
                continue;
              }
              this.fatal("decode: " + (err instanceof Error ? err.message : String(err)));
              return;
            }
          }
          console.warn("[WasmAudioEngine] segment decode failed, skipping:", err);
        } finally {
          this.inflight.delete(target);
        }
      }
    } finally {
      this.pumping = false;
    }
  }

  // scheduleWindow wires every already-decoded, not-yet-scheduled segment in the
  // look-ahead window onto the AudioContext clock. Segments still decoding are
  // picked up on a later tick once pump() has them.
  private scheduleWindow(ct: number): void {
    if (!this.anchored) return;
    const start = this.segmentIndexAt(ct);
    // Budget per tick. Scheduling a segment allocates an AudioBuffer for it (a
    // 3 s 5.1 segment is ~3.4 MB) and, with the planar worker protocol, fills
    // it with native copyToChannel calls - a cached old worker still on the
    // interleaved protocol instead de-interleaves ~864 K samples in a JS loop.
    // Either way, draining the whole 20 s window in one tick is a single task
    // doing ~24 MB of allocation (plus the JS loop on the legacy path). The
    // sound never suffers - it is already scheduled on the audio thread - but
    // the page cannot paint meanwhile, which is the "picture slows down while
    // the sound stays fluid" report. A resync empties scheduledIdx, so that
    // burst landed on exactly the moment the viewer resumed.
    //
    // Spreading it costs nothing: the window is 20 s ahead and the tick runs
    // every 100 ms, so even one segment per tick fills it far faster than
    // playback drains it.
    let budget = SCHEDULE_PER_TICK;
    for (let idx = start; idx < this.segments.length; idx++) {
      const seg = this.segments[idx];
      if (seg.start > ct + BUFFER_AHEAD_SEC) break;
      if (this.scheduledIdx.has(idx)) continue;
      if (seg.start + seg.dur <= ct - 0.05) continue; // wholly past
      const d = this.decodedCache.get(idx);
      if (!d) continue; // not decoded yet - a later tick schedules it
      this.scheduleDecoded(idx, seg, d);
      // The loop starts at the segment under the playhead, so with a budget of
      // at least one that segment is never deferred; the rest can wait a tick.
      if (--budget <= 0) break;
    }
  }

  private scheduleDecoded(idx: number, seg: Segment, d: DecodedSegment): void {
    const ctx = this.ctx;
    if (!ctx || !this.gain) return;
    let buffer: AudioBuffer;
    // baseOffset positions the playable content inside the buffer: the native
    // leg's preroll samples, plus the hand-over trim on the first AC-3 segment.
    let baseOffset = seg.trimSec ?? 0;
    if (d.kind === "pcm") {
      const mix = outputMix(d.channels);
      // The output layout is the mix's; a mixed planar reply already has
      // exactly that many channels.
      buffer = ctx.createBuffer(mix.length, d.frames, d.sampleRate);
      fillAudioBuffer(buffer, d, mix);
    } else {
      // decodeAudioData output is already in the browser's channel order.
      buffer = d.audioBuffer;
      baseOffset += d.offsetSec;
    }
    const trim = seg.trimSec ?? 0;
    const effDur = seg.dur - trim; // media seconds this source may occupy
    if (effDur <= 0) {
      this.scheduledIdx.add(idx);
      return;
    }
    const source = ctx.createBufferSource();
    source.buffer = buffer;
    source.playbackRate.value = this.anchorRate;
    source.connect(this.gain);

    const startAt = this.ctxForMedia(seg.start + trim) + this.leadComp + this.userOffsetSec;
    const now = ctx.currentTime;
    let late = 0;
    if (startAt >= now) {
      source.start(startAt, baseOffset);
    } else {
      // We are late for this segment (resync landed mid-segment, or the fetch
      // lagged): start immediately, offset into the buffer so the sample under
      // the playhead plays now. The buffer offset is in media seconds, so the
      // wall-clock lateness is scaled back up by the rate.
      late = (now - startAt) * this.anchorRate;
      if (late >= effDur) {
        return; // wholly in the past
      }
      source.start(now, baseOffset + late);
    }
    if (d.kind === "buffer") {
      // A natively decoded buffer can run a hair longer than its EXTINF
      // (preroll rounding, encoder tail): stop it AT the boundary so it can
      // never talk over the next segment's scheduled start.
      source.stop(Math.max(startAt, now) + (effDur - late) / this.anchorRate);
    }
    this.activeSources.add(source);
    this.scheduledIdx.add(idx);
    this.everScheduled = true;
    source.onended = () => {
      this.activeSources.delete(source);
    };
  }

  // debugStats reports the live sync state: the DEV Playwright harness and the
  // playback beacons read it. driftMs is the anchor's own view and is small by
  // construction; leadMs is the delay the schedule ADDS on top of it (measured
  // estimate, or the static base until it matures) - the figure a viewer's
  // "audio late by N ms" report has to be checked against.
  debugStats(): {
    videoTime: number;
    audioMediaTime: number;
    driftMs: number;
    leadMs: number;
    leadMeasured: boolean;
    userOffsetMs: number;
    outputLatencyMs: number;
    activeSources: number;
    anchored: boolean;
    ctxState: string;
    segments: number;
    decoded: number;
    mode: string;
    heals: number;
  } {
    const audioMediaTime = this.anchored
      ? this.anchorMedia + ((this.ctx?.currentTime ?? 0) - this.anchorCtx) * this.anchorRate
      : NaN;
    const ctx = this.ctx as (AudioContext & { outputLatency?: number }) | null;
    return {
      videoTime: this.video.currentTime,
      audioMediaTime,
      driftMs: (this.video.currentTime - audioMediaTime) * 1000,
      leadMs: this.leadComp * 1000,
      leadMeasured: this.leadEstimator.estimate != null,
      userOffsetMs: this.userOffsetSec * 1000,
      outputLatencyMs: (ctx?.outputLatency ?? ctx?.baseLatency ?? 0) * 1000,
      activeSources: this.activeSources.size,
      anchored: this.anchored,
      ctxState: this.ctx?.state ?? "none",
      segments: this.segments.length,
      decoded: this.decodedCache.size,
      mode: this.mode(),
      heals: this.heals,
    };
  }

  // heal records one audio-output intervention (audioHealth.ts) and reports
  // it: the beacon is what makes the next silent picture diagnosable, the
  // backoff is what keeps a heal that does not take from firing at tick rate.
  private heal(reason: string, now: number): void {
    this.heals++;
    this.nextHealAt = now + healBackoffMs(this.heals - 1);
    console.warn("[WasmAudioEngine] heal:", reason);
    this.opts.onDiagnostic?.(`${reason} (heal #${this.heals})`);
  }

  // --- teardown -----------------------------------------------------------

  destroy(): void {
    if (this.destroyed) return;
    this.destroyed = true;
    if (this.rvfcHandle !== null) {
      (this.video as HTMLVideoElement & { cancelVideoFrameCallback?: (h: number) => void })
        .cancelVideoFrameCallback?.(this.rvfcHandle);
      this.rvfcHandle = null;
    }
    if (this.timer) clearInterval(this.timer);
    this.timer = null;
    this.clearScheduled();
    this.decodedCache.clear();
    this.inflight.clear();
    for (const [ev, fn] of this.boundHandlers) this.video.removeEventListener(ev, fn);
    this.boundHandlers.length = 0;
    this.gestureUnlock?.();
    this.gestureUnlock = null;
    for (const p of this.pendingDecodes.values()) p.reject(new Error("engine destroyed"));
    this.pendingDecodes.clear();
    this.worker?.terminate();
    this.worker = null;
    void this.ctx?.close().catch(() => { /* already closing */ });
    this.ctx = null;
    this.gain = null;
  }
}
