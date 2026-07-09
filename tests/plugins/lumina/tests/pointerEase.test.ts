import { describe, it, expect } from "vitest";
import { createPointerState, stepPointer } from "../src/domain/services/pointerEase";

describe("pointer easing", () => {
  it("mutates the same object in place (no per-step allocation)", () => {
    const state = createPointerState();
    const ref = state;
    stepPointer(state, 1, -1, 0.1);
    expect(state).toBe(ref);
  });

  it("starts at rest", () => {
    expect(createPointerState()).toEqual({ x: 0, y: 0 });
  });

  it("zero dt leaves the state unchanged", () => {
    const state = createPointerState();
    stepPointer(state, 1, 1, 0);
    expect(state).toEqual({ x: 0, y: 0 });
  });

  it("negative dt is clamped (never moves backwards / throws)", () => {
    const state = { x: 0.4, y: -0.2 };
    stepPointer(state, 1, 1, -0.5);
    expect(state).toEqual({ x: 0.4, y: -0.2 });
  });

  it("eases monotonically closer to a fixed target every step", () => {
    const state = createPointerState();
    let prevDist = Math.hypot(1 - state.x, 1 - state.y);
    for (let i = 0; i < 30; i++) {
      stepPointer(state, 1, 1, 1 / 30);
      const dist = Math.hypot(1 - state.x, 1 - state.y);
      expect(dist).toBeLessThan(prevDist);
      prevDist = dist;
    }
  });

  it("converges arbitrarily close to the target given enough steps", () => {
    const state = createPointerState();
    for (let i = 0; i < 500; i++) stepPointer(state, -0.6, 0.9, 1 / 60);
    expect(state.x).toBeCloseTo(-0.6, 3);
    expect(state.y).toBeCloseTo(0.9, 3);
  });

  it("a lazy field never fully snaps to the target in a single large step", () => {
    const state = createPointerState();
    stepPointer(state, 1, 1, 1 / 60);
    expect(state.x).toBeGreaterThan(0);
    expect(state.x).toBeLessThan(1);
  });
});
