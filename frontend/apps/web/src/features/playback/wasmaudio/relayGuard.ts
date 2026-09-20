// Guards around the AAC -> wasm hand-over of the bi-source relay.
//
// The hand-over used to fire on a single condition: the wasm worker reported
// "ready". Ready is not the same as FAST ENOUGH. On a weak operator box casting an
// E-AC-3 7.1 track (8 channels to decode, then folded to 5.1) the decoder could
// not sustain real time: after the switch the engine fetched one audio segment
// every ~18s for ~5s of content, the sound died, and the receiver was killed by
// the box a couple of minutes later - twice (prod logs, 2026-07-30).
//
// Nothing caught it, because the only fallback (`aac-latched`) required the wasm
// leg to REPORT AN ERROR. A decoder that merely crawls raises nothing at all.
//
// Two guards, and they are deliberately separate: one refuses a hand-over that
// looks doomed BEFORE it happens, the other rescues playback when it goes wrong
// anyway. Pure functions - the engine holds the state, this file holds the
// judgement, and the judgement is what needs to be readable and tested.

/** A wasm decode is only worth switching to if it is comfortably faster than
 * real time. 0.7 leaves room for the CPU contention a TV box has when it is
 * also decoding video: a decoder that needs 70% of a segment's duration to
 * decode it has almost nothing left for a bad moment. */
export const DEFAULT_MAX_REALTIME_RATIO = 0.7;

/** Above 6 channels the decoder also pays for the 7.1 -> 5.1 fold on top of a
 * bigger decode. That is the exact configuration that failed in prod, and the
 * AAC leg renders it perfectly well. */
export const DEFAULT_MAX_CHANNELS = 6;

/** No decoded wasm segment for this long, while the picture is running, means
 * the leg is starving rather than merely slow. Generous on purpose: segments
 * are a few seconds and a single slow fetch must not trigger a fallback. */
export const DEFAULT_STARVATION_MS = 12_000;

export interface HandoverProbe {
  /** Wall time the probe decode took, in milliseconds. */
  decodeMs: number;
  /** Duration of the probed segment, in seconds. */
  segmentDurSec: number;
  /** Channel count the decoder reported. */
  channels: number;
  maxRealtimeRatio?: number;
  maxChannels?: number;
}

export interface HandoverVerdict {
  ok: boolean;
  /** Short, loggable reason when ok is false - it lands in the prod beacon, and
   * "why did my cast stay on AAC" must be answerable without a TV screenshot. */
  reason?: string;
}

/**
 * shouldHandover judges a probe decode of ONE real segment: can this device
 * carry the wasm leg?
 *
 * Undecidable inputs (a zero-length segment, a nonsense channel count) answer
 * NO: refusing a hand-over costs audio quality, accepting a bad one costs the
 * whole playback.
 */
export function shouldHandover(probe: HandoverProbe): HandoverVerdict {
  const maxRatio = probe.maxRealtimeRatio ?? DEFAULT_MAX_REALTIME_RATIO;
  const maxChannels = probe.maxChannels ?? DEFAULT_MAX_CHANNELS;

  if (!Number.isFinite(probe.segmentDurSec) || probe.segmentDurSec <= 0) {
    return { ok: false, reason: "probe: unusable segment duration" };
  }
  if (!Number.isFinite(probe.channels) || probe.channels <= 0) {
    return { ok: false, reason: "probe: unknown channel count" };
  }
  if (probe.channels > maxChannels) {
    return { ok: false, reason: `probe: ${probe.channels} channels above the ${maxChannels} this device is trusted with` };
  }
  if (!Number.isFinite(probe.decodeMs) || probe.decodeMs < 0) {
    return { ok: false, reason: "probe: no timing" };
  }
  const ratio = probe.decodeMs / 1000 / probe.segmentDurSec;
  if (ratio > maxRatio) {
    return { ok: false, reason: `probe: decode at ${ratio.toFixed(2)}x real time, over the ${maxRatio}x limit` };
  }
  return { ok: true };
}

export interface StarvationState {
  /** Has the hand-over already happened? Before it, the AAC leg is playing. */
  switched: boolean;
  /** Is the picture actually running? A paused player decodes nothing, legitimately. */
  playing: boolean;
  /** Milliseconds since the last SUCCESSFUL wasm segment decode. */
  sinceLastDecodeMs: number;
  /**
   * Does the wasm leg still OWE audio - a segment inside the look-ahead window
   * it has neither decoded nor scheduled? A leg that owes nothing cannot be
   * starving, however long ago its last decode was: that is the normal state
   * after a backward seek into a stretch the cache already holds.
   * Undefined keeps the old, blind behaviour.
   */
  pending?: boolean;
  thresholdMs?: number;
}

/**
 * isStarved reports that the wasm leg has stopped delivering while playback
 * continues - the silent-picture failure. Deliberately blind to WHY (slow
 * decode, stalled fetch, wedged worker): the answer is the same, hand the film
 * back to the AAC leg.
 *
 * It is NOT blind to "nothing was asked of it". Counting the seconds since the
 * last decode alone made a healthy leg look dead: on 2026-09-20 a viewer seeked
 * back into audio the engine had already decoded (the cache holds everything
 * ahead of the playhead, 35 segments that day), the pump found the whole window
 * covered, no decode completed for 32 s, and the film was handed to the AAC
 * transcode while the wasm leg was playing 4 scheduled segments at 0 ms drift.
 */
export function isStarved(state: StarvationState): boolean {
  if (!state.switched || !state.playing) return false;
  if (state.pending === false) return false;
  const threshold = state.thresholdMs ?? DEFAULT_STARVATION_MS;
  return Number.isFinite(state.sinceLastDecodeMs) && state.sinceLastDecodeMs > threshold;
}
