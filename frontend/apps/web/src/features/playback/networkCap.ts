// Bandwidth follower for the pinned-rung player (hls.js ABR is off on the
// transcode path). It measures the pipe from each segment's transfer time
// only - first byte to last - so a rafale that was slow to PRODUCE never
// reads as a slow line, which is what made free ABR collapse to 360p.

export interface ThroughputSample {
  bytes: number;
  transferMs: number;
  at: number;
  /** Media seconds the segment covers: what the rung REALLY costs, next to its declared bitrate. */
  durationSec?: number;
}

export interface NetworkLevel {
  height: number;
  kbps: number;
}

export interface NetworkCapState {
  samples: ThroughputSample[];
  stallsAt: number[];
  lastChangeAt: number;
}

export interface NetworkStep {
  index: number;
  height: number;
  direction: "down" | "up";
  estimateKbps: number;
}

const SAMPLE_WINDOW_MS = 60_000;
const MAX_SAMPLES = 8;
const MIN_SAMPLE_BYTES = 32 * 1024;
const MIN_SAMPLES = 3;
/** The rung must fit with this much headroom, or it is too big for the line. */
const DOWN_HEADROOM = 1.2;
/** Stepping up needs the next rung to fit comfortably. */
const UP_HEADROOM = 1.5;
const UP_MIN_BUFFER_SEC = 20;
const DOWN_COOLDOWN_MS = 20_000;
const UP_COOLDOWN_MS = 30_000;
const STALLS_TO_ACT = 2;

export function newNetworkCapState(): NetworkCapState {
  return { samples: [], stallsAt: [], lastChangeAt: 0 };
}

export function recordThroughput(s: NetworkCapState, sample: ThroughputSample): void {
  if (sample.bytes < MIN_SAMPLE_BYTES || sample.transferMs < 5) return;
  s.samples.push(sample);
  s.samples = s.samples.filter((x) => sample.at - x.at <= SAMPLE_WINDOW_MS).slice(-MAX_SAMPLES);
}

export function recordStall(s: NetworkCapState, now: number): void {
  s.stallsAt.push(now);
  s.stallsAt = s.stallsAt.filter((t) => now - t <= SAMPLE_WINDOW_MS);
}

/** Bytes over time across the window: one slow segment cannot dominate. */
export function estimateKbps(s: NetworkCapState): number | null {
  if (s.samples.length < MIN_SAMPLES) return null;
  let bytes = 0;
  let ms = 0;
  for (const x of s.samples) {
    bytes += x.bytes;
    ms += x.transferMs;
  }
  return ms > 0 ? (bytes * 8) / ms : null;
}

/** The heaviest recent segment, in kbps of media: a rung whose encoder bursts
 * above its declared bitrate (70 Mb/s declared, 3 s segments at 100+ Mb/s,
 * prod 2026-09-11) is judged on what it ships. 0 when no sample says. */
export function observedRungKbps(s: NetworkCapState): number {
  let max = 0;
  for (const x of s.samples) {
    if (x.durationSec && x.durationSec > 0) max = Math.max(max, (x.bytes * 8) / x.durationSec / 1000);
  }
  return max;
}

export function decideNetworkStep(
  s: NetworkCapState,
  now: number,
  levels: readonly NetworkLevel[],
  currentIndex: number,
  ceilingIndex: number,
  bufferAheadSec: number,
): NetworkStep | null {
  const est = estimateKbps(s);
  if (est == null || currentIndex < 0 || currentIndex >= levels.length) return null;
  const current = levels[currentIndex];
  const need = Math.max(current.kbps, observedRungKbps(s));

  if (currentIndex > 0 && est < need * DOWN_HEADROOM && now - s.lastChangeAt >= DOWN_COOLDOWN_MS) {
    let target = 0;
    for (let i = currentIndex - 1; i >= 0; i--) {
      if (levels[i].kbps * DOWN_HEADROOM <= est) {
        target = i;
        break;
      }
    }
    s.lastChangeAt = now;
    s.stallsAt = [];
    return { index: target, height: levels[target].height, direction: "down", estimateKbps: est };
  }

  // Repeated stalls with a line that only just fits: do not wait for the window.
  if (currentIndex > 0 && s.stallsAt.length >= STALLS_TO_ACT && est < need * UP_HEADROOM && now - s.lastChangeAt >= DOWN_COOLDOWN_MS) {
    s.lastChangeAt = now;
    s.stallsAt = [];
    const target = currentIndex - 1;
    return { index: target, height: levels[target].height, direction: "down", estimateKbps: est };
  }

  const next = currentIndex + 1;
  if (
    next <= ceilingIndex &&
    next < levels.length &&
    est > levels[next].kbps * UP_HEADROOM &&
    bufferAheadSec >= UP_MIN_BUFFER_SEC &&
    s.stallsAt.length === 0 &&
    now - s.lastChangeAt >= UP_COOLDOWN_MS
  ) {
    s.lastChangeAt = now;
    return { index: next, height: levels[next].height, direction: "up", estimateKbps: est };
  }
  return null;
}

export function formatNetworkStep(step: NetworkStep, fromHeight: number): string {
  return `${fromHeight}p->${step.height}p est=${Math.round(step.estimateKbps)}kbps ${step.direction}`;
}
