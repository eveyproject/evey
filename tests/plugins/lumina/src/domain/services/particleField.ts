// Bounded particle field: the floating "dust/bokeh" specks. Positions/sizes
// are seeded once (pure, deterministic hash - no Math.random so the field is
// reproducible/testable); the actual drift + pointer-parallax motion happens
// in the vertex shader from uTime/uPointer (see adapters/render/shaders.ts),
// so there is nothing to allocate or mutate on the CPU every frame.

export interface ParticleSeed {
  baseX: number; // -1..1
  baseY: number; // -1..1
  depth: number; // 0..1, 0 = far (small, slow parallax), 1 = near
  size: number; // relative point-size multiplier
  phase: number; // radians, per-particle motion offset
  speed: number; // per-particle motion-rate multiplier
}

export type QualityTier = "low" | "mid" | "high";

// Bounded budget per tier - a deliberately small budget for a background
// wallpaper effect (the real cost lives in the fullscreen veil shader, not
// in point count): the low/weak-NAS floor never exceeds 14 points, the top
// tier caps at 40.
const BUDGET: Record<QualityTier, number> = { low: 14, mid: 26, high: 40 };
export const MAX_PARTICLES = BUDGET.high;

export function particleBudgetForTier(tier: QualityTier): number {
  return BUDGET[tier];
}

// Deterministic hash -> [0, 1). Same shape as the shaders' hash21, kept
// separate/CPU-side so the seed generation needs no GL context to test.
function hash(n: number): number {
  const s = Math.sin(n) * 43758.5453123;
  return s - Math.floor(s);
}

export function createParticleField(count: number, seed: number): ParticleSeed[] {
  const n = Math.max(0, Math.min(Math.floor(count), MAX_PARTICLES));
  const out: ParticleSeed[] = new Array(n);
  for (let i = 0; i < n; i++) {
    const base = seed + i * 12.9898;
    out[i] = {
      baseX: hash(base * 1.1) * 2 - 1,
      baseY: hash(base * 2.3 + 7.1) * 2 - 1,
      depth: hash(base * 3.7 + 4.2),
      size: 0.4 + hash(base * 5.9 + 1.3) * 0.6,
      phase: hash(base * 7.7 + 2.6) * Math.PI * 2,
      speed: 0.15 + hash(base * 9.1 + 0.4) * 0.35,
    };
  }
  return out;
}
