// Continuous per-frame animation phases fed to the shader as time-like
// uniforms. A single mutable object, allocated once at mount and mutated in
// place every frame by the renderer - never a per-frame literal. Each phase
// wraps well below float32/mediump precision loss so a widget left open for
// hours never drifts or jitters (same discipline as weather-v2's
// TIME_WRAP_S).

export interface AnimationState {
  time: number; // drives the veil/fbm domain warp
  drift: number; // slow horizontal drift of the veil layers
  causticPhase: number; // drives the caustic liquid-light pattern
  vignettePulse: number; // slow vignette "breathing"
}

const TIME_WRAP_S = 4096;
const DRIFT_WRAP = 512;

export function createAnimationState(): AnimationState {
  return { time: 0, drift: 0, causticPhase: 0, vignettePulse: 0 };
}

export function stepAnimation(state: AnimationState, dt: number, speedMul: number): void {
  const step = Math.max(0, dt) * speedMul;
  state.time = (state.time + step) % TIME_WRAP_S;
  state.drift = (state.drift + step * 0.10) % DRIFT_WRAP;
  state.causticPhase = (state.causticPhase + step * 0.42) % TIME_WRAP_S;
  state.vignettePulse = (state.vignettePulse + step * 0.06) % TIME_WRAP_S;
}
