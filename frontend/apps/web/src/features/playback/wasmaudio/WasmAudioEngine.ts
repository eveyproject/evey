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

const WASM_URL = "/wasm/ac3go.wasm";
const WASM_EXEC_URL = "/wasm/wasm_exec.js";
const WORKER_URL = "/wasm/ac3Worker.js";

// How far ahead of the playhead to keep audio decoded+scheduled. Generous: the
// decoder is far faster than real time, and a deep buffer rides out a stalled
// segment fetch without an audible gap.
const BUFFER_AHEAD_SEC = 20;
// Re-anchor when the audio clock and the video clock disagree by more than this.
// Well above the natural ~0.1 ms/s drift (measured negligible) so only a seek or
// a stall recovery trips it - a re-anchor is a brief audio reset, not for free.
const RESYNC_THRESHOLD_SEC = 0.3;
// Constant lead compensation: the audio otherwise starts ~100 ms ahead of the
// picture (decoder has no output latency; the display + hls.js pipeline does).
// Delaying the audio by roughly the output latency lines them up. Tunable.
const DEFAULT_LEAD_COMP_SEC = 0.1;
const SCHEDULE_TICK_MS = 100;

interface Segment {
  url: string; // resolved, absolute
  start: number; // media start time (s)
  dur: number; // EXTINF (s)
}

interface DecodedSegment {
  channels: number;
  sampleRate: number;
  frames: number;
  pcm: Float32Array; // interleaved
}

export interface WasmAudioEngineOptions {
  // resolveUrl rewrites a URL with the freshest auth token before every fetch,
  // mirroring hls.js's xhrSetup so a long buffer-ahead can't 401 mid-film.
  resolveUrl?: (url: string) => string;
  // onFatal is called when the engine cannot continue (wasm load failed, the
  // playlist is unreadable, decode errors on the very first segments). The host
  // should tear the engine down and fall back to the AAC transcode path.
  onFatal?: (reason: string) => void;
  leadCompensationSec?: number;
}

// outputMix maps each Web Audio OUTPUT channel to the decoder source channels to
// sum into it (each a [sourceIndex, gain] tap). It puts the decoder's layout
// into Web Audio's "speakers" order, and folds 7.1 down to 5.1: Web Audio only
// applies its speaker down-mix (the one that keeps the centre on a stereo output)
// up to 5.1 - an 8-channel buffer falls back to "discrete", which on a stereo
// device drops every channel past L/R, i.e. the dialogue. Folding to 5.1 keeps
// the browser's correct down-mix AND preserves the 7.1 back-channel content
// (mixed into the surrounds) instead of dropping it. Exported for unit testing.
//   5.1 decoder L,C,R,Ls,Rs,LFE          -> WA L,R,C,LFE,SL,SR
//   7.1 decoder L,R,C,LFE,BL,BR,SL,SR     -> WA 5.1, SL=SL+BL, SR=SR+BR
export function outputMix(channels: number): Array<Array<[number, number]>> {
  if (channels === 6) {
    return [[[0, 1]], [[2, 1]], [[1, 1]], [[5, 1]], [[3, 1]], [[4, 1]]];
  }
  if (channels === 8) {
    return [[[0, 1]], [[1, 1]], [[2, 1]], [[3, 1]], [[6, 1], [4, 1]], [[7, 1], [5, 1]]];
  }
  return Array.from({ length: channels }, (_, i) => [[i, 1]] as Array<[number, number]>);
}

export class WasmAudioEngine {
  private readonly video: HTMLVideoElement;
  private readonly playlistUrl: string;
  private readonly opts: WasmAudioEngineOptions;
  private readonly leadComp: number;

  private worker: Worker | null = null;
  private ctx: AudioContext | null = null;
  private gain: GainNode | null = null;

  private segments: Segment[] = [];

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
    this.leadComp = opts.leadCompensationSec ?? DEFAULT_LEAD_COMP_SEC;
  }

  private resolve(url: string): string {
    return this.opts.resolveUrl ? this.opts.resolveUrl(url) : url;
  }

  private fatal(reason: string): void {
    if (this.destroyed) return;
    console.warn("[WasmAudioEngine] fatal:", reason);
    this.opts.onFatal?.(reason);
  }

  async start(): Promise<void> {
    if (this.started || this.destroyed) return;
    this.started = true;
    try {
      await this.loadPlaylist();
      if (this.destroyed) return;
      await this.initWorker();
      if (this.destroyed) return;
      this.initAudio();
      this.attachVideoEvents();
      this.startScheduling();
    } catch (err) {
      this.fatal(err instanceof Error ? err.message : String(err));
    }
  }

  // --- playlist -----------------------------------------------------------

  private async loadPlaylist(): Promise<void> {
    const resp = await fetch(this.resolve(this.playlistUrl), { credentials: "include" });
    if (!resp.ok) throw new Error(`audio playlist ${resp.status}`);
    const text = await resp.text();
    const base = new URL(this.playlistUrl, window.location.origin);
    const segs: Segment[] = [];
    let start = 0;
    let pendingDur = 0;
    for (const raw of text.split("\n")) {
      const line = raw.trim();
      if (!line) continue;
      if (line.startsWith("#EXTINF:")) {
        pendingDur = parseFloat(line.slice("#EXTINF:".length)) || 0;
        continue;
      }
      if (line.startsWith("#")) continue; // EXT-X-MAP etc. - the init rides EXT-X-MAP, which the demuxer does not need per-segment (samples are self-describing)
      const url = new URL(line, base).toString();
      segs.push({ url, start, dur: pendingDur });
      start += pendingDur;
      pendingDur = 0;
    }
    if (segs.length === 0) throw new Error("audio playlist has no segments");
    this.segments = segs;
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
          const p = this.pendingDecodes.get(msg.id);
          if (p) {
            this.pendingDecodes.delete(msg.id);
            p.resolve({
              channels: msg.channels,
              sampleRate: msg.sampleRate,
              frames: msg.frames,
              pcm: new Float32Array(msg.pcm),
            });
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
      worker.postMessage({ type: "init", wasmUrl: WASM_URL, wasmExecUrl: WASM_EXEC_URL });
    });
  }

  private decodeSegment(seg: Segment): Promise<DecodedSegment> {
    return new Promise<DecodedSegment>((resolve, reject) => {
      const worker = this.worker;
      if (!worker) {
        reject(new Error("no worker"));
        return;
      }
      fetch(this.resolve(seg.url), { credentials: "include" })
        .then((r) => {
          if (!r.ok) throw new Error(`segment ${r.status}`);
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
    if (!ctx || ctx.state !== "suspended" || !this.ctxRetry) return;
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

  private startScheduling(): void {
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
      }
      return;
    }

    this.resumeCtx();
    if (this.ctx.state === "suspended") return; // not unlocked yet - wait for a gesture
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
    if (this.pumping || this.destroyed || !this.worker) return;
    this.pumping = true;
    try {
      while (!this.destroyed) {
        const ct = this.video.currentTime;
        // Evict played-out segments (memory bound).
        for (const idx of this.decodedCache.keys()) {
          const s = this.segments[idx];
          if (s && s.start + s.dur < ct - 4) this.decodedCache.delete(idx);
        }
        // First not-yet-decoded segment in the window, measured from the LIVE ct.
        let target = -1;
        for (let idx = this.segmentIndexAt(ct); idx < this.segments.length; idx++) {
          const seg = this.segments[idx];
          if (seg.start > ct + BUFFER_AHEAD_SEC) break;
          if (seg.start + seg.dur <= ct - 0.05) continue; // wholly past
          if (this.decodedCache.has(idx) || this.inflight.has(idx)) continue;
          target = idx;
          break;
        }
        if (target < 0) break; // window fully decoded
        const seg = this.segments[target];
        this.inflight.add(target);
        try {
          const d = await this.decodeSegment(seg);
          if (this.destroyed) return;
          this.decodedCache.set(target, d);
          this.decodeFailStreak = 0;
        } catch (err) {
          // A single bad segment once playback is going must not kill it. But if
          // NOTHING has ever scheduled and decodes keep failing, the stream is
          // undecodable - fall back to the AAC transcode.
          if (!this.everScheduled) {
            this.decodeFailStreak++;
            if (this.decodeFailStreak >= 3) {
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
    for (let idx = start; idx < this.segments.length; idx++) {
      const seg = this.segments[idx];
      if (seg.start > ct + BUFFER_AHEAD_SEC) break;
      if (this.scheduledIdx.has(idx)) continue;
      if (seg.start + seg.dur <= ct - 0.05) continue; // wholly past
      const d = this.decodedCache.get(idx);
      if (!d) continue; // not decoded yet - a later tick schedules it
      this.scheduleDecoded(idx, seg, d);
    }
  }

  private scheduleDecoded(idx: number, seg: Segment, d: DecodedSegment): void {
    const ctx = this.ctx;
    if (!ctx || !this.gain) return;
    const mix = outputMix(d.channels);
    const ch = d.channels;
    const buffer = ctx.createBuffer(mix.length, d.frames, d.sampleRate);
    for (let co = 0; co < mix.length; co++) {
      const out = buffer.getChannelData(co);
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
    const source = ctx.createBufferSource();
    source.buffer = buffer;
    source.playbackRate.value = this.anchorRate;
    source.connect(this.gain);

    const startAt = this.ctxForMedia(seg.start) + this.leadComp;
    const now = ctx.currentTime;
    if (startAt >= now) {
      source.start(startAt);
    } else {
      // We are late for this segment (resync landed mid-segment, or the fetch
      // lagged): start immediately, offset into the buffer so the sample under
      // the playhead plays now. The buffer offset is in media seconds, so the
      // wall-clock lateness is scaled back up by the rate.
      const into = (now - startAt) * this.anchorRate;
      if (into >= seg.dur) {
        return; // wholly in the past
      }
      source.start(now, into);
    }
    this.activeSources.add(source);
    this.scheduledIdx.add(idx);
    this.everScheduled = true;
    source.onended = () => {
      this.activeSources.delete(source);
    };
  }

  // debugStats reports the live sync state. Used only by the DEV Playwright
  // harness to measure real A/V drift; carries no production behaviour.
  debugStats(): {
    videoTime: number;
    audioMediaTime: number;
    driftMs: number;
    activeSources: number;
    anchored: boolean;
    ctxState: string;
    segments: number;
    decoded: number;
  } {
    const audioMediaTime = this.anchored
      ? this.anchorMedia + ((this.ctx?.currentTime ?? 0) - this.anchorCtx) * this.anchorRate
      : NaN;
    return {
      videoTime: this.video.currentTime,
      audioMediaTime,
      driftMs: (this.video.currentTime - audioMediaTime) * 1000,
      activeSources: this.activeSources.size,
      anchored: this.anchored,
      ctxState: this.ctx?.state ?? "none",
      segments: this.segments.length,
      decoded: this.decodedCache.size,
    };
  }

  // --- teardown -----------------------------------------------------------

  destroy(): void {
    if (this.destroyed) return;
    this.destroyed = true;
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
