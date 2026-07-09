// Pure time -> palette function: the "premium evolving palette" the scene is
// built around. Given a preset and how many milliseconds the widget has been
// open, returns the full set of colors the shader needs for this instant.
// Deterministic and allocation-cheap (5 short tuples), safe to call every
// frame from the renderer - see adapters/render/GlRenderer.ts.

import type { PaletteFrame, PresetDef, RGB } from "../model/scene";

// A full loop through every anchor takes this long. "Several minutes" per
// the spec; chosen so the drift is perceptible but never rushed.
export const PALETTE_CYCLE_MS = 6 * 60_000;

// veilB runs a constant fraction of a loop ahead of veilA so the two veils
// are always visibly out of phase (never carry the same color at once),
// which is what makes them read as "crossing" rather than moving together.
const VEIL_B_PHASE = 0.37;
const VEIL_C_PHASE = -0.19;
const VEIL_C_DARKEN = 0.32;
const CAUSTIC_LIFT = 0.55; // lerp veilA toward white for the caustic tint

function smoothstep01(t: number): number {
  return t * t * (3 - 2 * t);
}

function lerp3(a: RGB, b: RGB, t: number): RGB {
  return [a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t, a[2] + (b[2] - a[2]) * t];
}

function darken(c: RGB, amount: number): RGB {
  return [c[0] * amount, c[1] * amount, c[2] * amount];
}

function lift(c: RGB, amount: number): RGB {
  return [c[0] + (1 - c[0]) * amount, c[1] + (1 - c[1]) * amount, c[2] + (1 - c[2]) * amount];
}

// Samples a looping gradient through `anchors` at phase `t` (in "loops",
// any real number - negative and >1 both wrap correctly).
function sampleAnchors(anchors: RGB[], t: number): RGB {
  const n = anchors.length;
  const wrapped = ((t % 1) + 1) % 1; // into [0, 1)
  const pos = wrapped * n;
  const i0 = Math.floor(pos) % n;
  const i1 = (i0 + 1) % n;
  const frac = pos - Math.floor(pos);
  return lerp3(anchors[i0], anchors[i1], smoothstep01(frac));
}

export function computePaletteFrame(preset: PresetDef, elapsedMs: number): PaletteFrame {
  const t = elapsedMs / PALETTE_CYCLE_MS;
  const veilA = sampleAnchors(preset.anchors, t);
  const veilB = sampleAnchors(preset.anchors, t + VEIL_B_PHASE);
  const veilC = darken(sampleAnchors(preset.anchors, t + VEIL_C_PHASE), VEIL_C_DARKEN);
  const caustic = lift(veilA, CAUSTIC_LIFT);
  return { veilA, veilB, veilC, caustic, vignette: preset.vignette };
}
