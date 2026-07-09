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
  // Wall-clock length of the whole spin. Bounded and short by design: the
  // geometric intervals below are NORMALIZED to this, so the spin always lands
  // in a fixed ~2.6 s instead of the old unbounded exponential growth (which
  // pushed the final steps minutes/hours out - the "selecting forever" bug).
  totalDurationMs?: number;
  growthFactor?: number;
}

const EMPTY_SPIN: RouletteSpin = { winnerIndex: -1, steps: [], totalDurationMs: 0 };

export function planRouletteSpin(candidateCount: number, seed: number, opts: RouletteOptions = {}): RouletteSpin {
  const count = Math.max(0, Math.floor(candidateCount));
  if (count === 0) return EMPTY_SPIN;

  const minLoops = opts.minLoops ?? 1;
  const maxLoops = opts.maxLoops ?? 2;
  const growth = opts.growthFactor ?? 1.08;
  const targetMs = opts.totalDurationMs ?? 2600;

  const rng = mulberry32(seed);
  const winnerIndex = Math.floor(rng() * count);
  const loops = minLoops + Math.floor(rng() * (maxLoops - minLoops + 1));
  // Total ticks so the last one lands exactly on winnerIndex, after `loops`
  // full laps around the candidates (a spinning-wheel feel, not a teleport).
  const totalSteps = loops * count + winnerIndex + 1;

  // Geometric intervals (fast start -> slow settle) computed in arbitrary
  // units, then normalized so the LAST step lands at exactly targetMs. This
  // keeps the decelerating feel but bounds the total to a fixed, short spin.
  const raw: number[] = [];
  let acc = 0;
  let interval = 1;
  for (let n = 0; n < totalSteps; n++) {
    raw.push(acc);
    acc += interval;
    interval *= growth;
  }
  const span = raw[raw.length - 1] || 1;

  const steps: RouletteStep[] = [];
  let prev = -1;
  for (let n = 0; n < totalSteps; n++) {
    let atMs = Math.round((targetMs * raw[n]) / span);
    if (atMs <= prev) atMs = prev + 1; // guarantee strictly increasing after rounding
    prev = atMs;
    steps.push({ atMs, index: n % count });
  }

  return { winnerIndex, steps, totalDurationMs: prev };
}
