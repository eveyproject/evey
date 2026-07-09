import { describe, it, expect } from "vitest";
import { createAnimationState, stepAnimation } from "../src/domain/services/animationState";

describe("animation state", () => {
  it("starts at zero", () => {
    expect(createAnimationState()).toEqual({ time: 0, drift: 0, causticPhase: 0, vignettePulse: 0 });
  });

  it("mutates the same object in place (no per-frame allocation)", () => {
    const state = createAnimationState();
    const ref = state;
    stepAnimation(state, 0.016, 1);
    expect(state).toBe(ref);
  });

  it("time advances by dt * speedMul each step", () => {
    const state = createAnimationState();
    stepAnimation(state, 0.5, 2);
    expect(state.time).toBeCloseTo(1.0, 10);
  });

  it("doubling speedMul doubles the per-step advance", () => {
    const a = createAnimationState();
    const b = createAnimationState();
    stepAnimation(a, 1, 1);
    stepAnimation(b, 1, 2);
    expect(b.time).toBeCloseTo(a.time * 2, 10);
    expect(b.drift).toBeCloseTo(a.drift * 2, 10);
  });

  it("negative dt is clamped to zero (never moves backwards)", () => {
    const state = createAnimationState();
    stepAnimation(state, -5, 1);
    expect(state).toEqual({ time: 0, drift: 0, causticPhase: 0, vignettePulse: 0 });
  });

  it("time wraps so a widget left open for hours never overflows", () => {
    const state = createAnimationState();
    // Push well past the wrap boundary in one step.
    stepAnimation(state, 5000, 1);
    expect(state.time).toBeGreaterThanOrEqual(0);
    expect(state.time).toBeLessThan(4096);
  });

  it("each phase advances at its own distinct rate", () => {
    const state = createAnimationState();
    stepAnimation(state, 1, 1);
    const values = [state.time, state.drift, state.causticPhase, state.vignettePulse];
    expect(new Set(values.map((v) => v.toFixed(6))).size).toBe(4);
  });
});
