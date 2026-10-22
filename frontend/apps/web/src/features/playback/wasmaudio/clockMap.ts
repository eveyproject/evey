// clockMap is the engine's mapping between the picture's media time and the
// AudioContext clock: where a media instant is scheduled, and which media
// instant the context is playing now. It used to be one line (an anchor and a
// rate), so the only answer to a drift was to drop the schedule and re-anchor -
// an audible cut, kept for drifts past 300 ms. Pieces let the rate bend from a
// media instant on, continuously, which is how a slow drift is taken back in
// silence (see driftCorrection).
//
// Pure data on plain numbers, so the junction arithmetic is tested on its own.

export interface ClockPiece {
  ctx: number; // context time at which the piece starts
  media: number; // media time at that instant
  rate: number; // media seconds per context second from there on
}

export class ClockMap {
  private pieces: ClockPiece[] = [];

  reset(ctx: number, media: number, rate: number): void {
    this.pieces = [{ ctx, media, rate }];
  }

  clear(): void {
    this.pieces = [];
  }

  get anchored(): boolean {
    return this.pieces.length > 0;
  }

  private byMedia(media: number): ClockPiece {
    let p = this.pieces[0];
    for (const q of this.pieces) {
      if (q.media <= media) p = q;
      else break;
    }
    return p;
  }

  private byCtx(ctx: number): ClockPiece {
    let p = this.pieces[0];
    for (const q of this.pieces) {
      if (q.ctx <= ctx) p = q;
      else break;
    }
    return p;
  }

  ctxForMedia(media: number): number {
    const p = this.byMedia(media);
    return p.ctx + (media - p.media) / p.rate;
  }

  mediaAtCtx(ctx: number): number {
    const p = this.byCtx(ctx);
    return p.media + (ctx - p.ctx) * p.rate;
  }

  rateForMedia(media: number): number {
    return this.byMedia(media).rate;
  }

  // bendFrom runs the clock at rate from media instant `media` on. The new
  // piece starts where the previous one puts that instant, so audio scheduled
  // up to it and audio scheduled after it meet with no gap and no overlap.
  bendFrom(media: number, rate: number): void {
    const ctx = this.ctxForMedia(media);
    this.pieces = this.pieces.filter((p) => p.media < media);
    this.pieces.push({ ctx, media, rate });
  }

  // pendingAt: a bend that the context has not reached yet. Its effect on the
  // drift is not measurable before it plays, so no further bend is decided.
  pendingAt(ctx: number): boolean {
    return this.pieces.some((p) => p.ctx > ctx);
  }

  // prune drops the pieces the context has left behind for good.
  prune(ctx: number): void {
    while (this.pieces.length > 1 && this.pieces[1].ctx <= ctx) this.pieces.shift();
  }
}

// Soft drift correction: past DRIFT_SOFT_START_SEC of median drift, the
// sound runs DRIFT_SOFT_RATE faster or slower (1.7 cents, inaudible) until the
// drift falls back under DRIFT_SOFT_STOP_SEC. A bend takes effect at the end
// of what is already scheduled (the 20 s look-ahead), so the stop sits well
// above zero: the stretch still queued carries the rest. 0.1 % takes back
// 1 ms a second, five times the 200 ppm the clocks were measured apart by in
// Chrome (2026-10-22). Past RESYNC_THRESHOLD_SEC the hard re-anchor still acts.
export const DRIFT_SOFT_START_SEC = 0.04;
export const DRIFT_SOFT_STOP_SEC = 0.015;
export const DRIFT_SOFT_RATE = 0.001;

// driftCorrection decides the correction from the median drift (picture minus
// sound, seconds) and the one in force: +1 speeds the sound up (it is behind
// the picture), -1 slows it down, 0 lets it run.
export function driftCorrection(driftSec: number, current: -1 | 0 | 1): -1 | 0 | 1 {
  if (!Number.isFinite(driftSec)) return current;
  const dir = driftSec > 0 ? 1 : -1;
  if (current === 0) return Math.abs(driftSec) > DRIFT_SOFT_START_SEC ? dir : 0;
  // Overshot to the other side, or close enough: stop.
  if (dir !== current || Math.abs(driftSec) < DRIFT_SOFT_STOP_SEC) return 0;
  return current;
}
