// The "Ce soir, on regarde ?" moment: a fully precomputed spin schedule (the
// spotlight sweeps candidate by candidate, slowing down, landing on
// `winnerIndex`). Precomputing the whole schedule up front - rather than
// picking a random index once the animation stops - keeps the render loop
// allocation-free (it just walks a fixed array by elapsed time) and makes the
// whole moment a pure, seed-reproducible function for tests.

import { mulberry32 } from "./prng";

export interface RouletteStep {
  atMs: number;
  index: number;
}

export interface RouletteSpin {
  winnerIndex: number;
  steps: RouletteStep[];
  totalDurationMs: number;
}

export interface RouletteOptions {
  minLoops?: number;
  maxLoops?: number;
  startIntervalMs?: number;
  minIntervalMs?: number;
  growthFactor?: number;
}

const EMPTY_SPIN: RouletteSpin = { winnerIndex: -1, steps: [], totalDurationMs: 0 };

export function planRouletteSpin(candidateCount: number, seed: number, opts: RouletteOptions = {}): RouletteSpin {
  const count = Math.max(0, Math.floor(candidateCount));
  if (count === 0) return EMPTY_SPIN;

  const minLoops = opts.minLoops ?? 2;
  const maxLoops = opts.maxLoops ?? 4;
  const growth = opts.growthFactor ?? 1.15;
  const minInterval = opts.minIntervalMs ?? 70;
  const startInterval = Math.max(minInterval, opts.startIntervalMs ?? 70);

  const rng = mulberry32(seed);
  const winnerIndex = Math.floor(rng() * count);
  const loops = minLoops + Math.floor(rng() * (maxLoops - minLoops + 1));
  // Total ticks so the last one lands exactly on winnerIndex, after `loops`
  // full laps around the candidates (a spinning-wheel feel, not a teleport).
  const totalSteps = loops * count + winnerIndex + 1;

  const steps: RouletteStep[] = [];
  let t = 0;
  let interval = startInterval;
  for (let n = 0; n < totalSteps; n++) {
    steps.push({ atMs: Math.round(t), index: n % count });
    const progress = n / totalSteps;
    // Deceleration ramps up hard in the last ~45% of the spin so it visibly
    // "settles" on the winner instead of stopping abruptly.
    const decel = progress > 0.55 ? growth + (progress - 0.55) * 1.8 : growth;
    interval = Math.max(minInterval, interval * decel);
    t += interval;
  }

  return { winnerIndex, steps, totalDurationMs: Math.round(t) };
}
