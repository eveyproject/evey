// Audio-output health for WasmAudioEngine: the silent-picture failures the
// stall gate and the starvation guard cannot see, because in both the picture
// keeps advancing and the leg keeps decoding - only the AudioContext has
// stopped making sound.
//
// Prod, 2026-09-21 (a phone browser, an app freeze mid-film): the viewer lost
// the sound while the picture ran, and a 30 s seek back brought it straight
// back. Nothing in the beacons could show it: every snapshot with a suspended
// context also had a frozen picture. Two ways a context stops sounding while
// the picture runs, both of which a seek happens to cure (a seek cycles the
// context through suspend/resume):
//   - stuck suspended: resumeCtx() disarms itself after ONE resume() call and
//     only a gesture re-arms it; if that call never lands (a browser that
//     refuses it after a background freeze), the context stays suspended and
//     the tick returns early forever;
//   - frozen clock: the context reads "running" but its currentTime no longer
//     advances - the output device died under it (audio focus, a frozen
//     process). Sources get scheduled onto a clock that never reaches them.
// Pure judgement, timestamps injected: the engine holds the state.

/** How long the context may stay suspended, or its clock frozen, while the
 * picture advances before the engine intervenes. Long enough that a resume()
 * in flight, or a context still spinning up its output, never trips it. */
export const AUDIO_STUCK_MS = 3_000;

/** Backoff between two heal attempts. A heal that does not take is retried,
 * just not at tick rate: a browser refusing resume() without a gesture logs a
 * warning per call. */
export const AUDIO_HEAL_BACKOFF_MS = [3_000, 6_000, 12_000, 30_000];

export interface ClockSample {
  /** Wall time (ms) at which the context clock was last seen moving. */
  at: number;
  /** The context clock (s) at that moment. */
  ctxTime: number;
}

/**
 * audioClockFrozen judges the context clock against wall time. The sample is
 * re-based whenever the clock moves, so `frozen` means "not a millisecond of
 * audio rendered for stuckMs of wall time".
 */
export function audioClockFrozen(
  prev: ClockSample | null,
  now: number,
  ctxTime: number,
  stuckMs = AUDIO_STUCK_MS,
): { frozen: boolean; sample: ClockSample } {
  if (!prev || ctxTime > prev.ctxTime + 1e-3) return { frozen: false, sample: { at: now, ctxTime } };
  return { frozen: now - prev.at > stuckMs, sample: prev };
}

/** stuckFor: how long a condition (suspended while advancing) has held, given
 * when it was first seen; 0 when it is not holding. */
export function stuckFor(since: number | null, now: number): number {
  return since == null ? 0 : Math.max(0, now - since);
}

/** healBackoffMs: the wait before heal attempt number `heals + 1`, after
 * `heals` attempts so far. */
export function healBackoffMs(heals: number): number {
  const i = Math.min(Math.max(0, heals), AUDIO_HEAL_BACKOFF_MS.length - 1);
  return AUDIO_HEAL_BACKOFF_MS[i];
}
