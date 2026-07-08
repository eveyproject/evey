import { describe, it, expect } from "vitest";
import { initThunderState, stepThunder } from "../src/domain/services/thunder";

function seqRand(values: number[]): () => number {
  let i = 0;
  return () => values[Math.min(i++, values.length - 1)];
}

const FRAME_SEC = 1 / 30;

describe("stepThunder", () => {
  it("calm sky: flash stays at zero and the RNG is never consulted", () => {
    const rand = () => {
      throw new Error("rand should not be called when thunderRate is 0");
    };
    const s = initThunderState();
    stepThunder(s, FRAME_SEC, 0, rand);
    expect(s.flash).toBe(0);
    expect(s.countdown).toBe(0);
  });

  it("schedules a flash immediately when the countdown has elapsed", () => {
    const rand = seqRand([0.4, 0.25, 0.6, 0.9]);
    const s = initThunderState();
    stepThunder(s, FRAME_SEC, 0.5, rand);
    // 0.7 + 0.4*0.5 = 0.9, minus one frame of decay.
    expect(s.flash).toBeCloseTo(0.9 - FRAME_SEC * 5, 5);
    expect(s.flashX).toBeGreaterThanOrEqual(0.15);
    expect(s.flashX).toBeLessThanOrEqual(0.85);
    expect(s.flashY).toBeGreaterThanOrEqual(0.05);
    expect(s.flashY).toBeLessThanOrEqual(0.40);
    expect(s.countdown).toBeGreaterThan(0);
  });

  it("flash decays linearly while waiting for the next scheduled strike", () => {
    const armed = { flash: 0.8, countdown: 5, flashX: 0.5, flashY: 0.2 };
    const rand = () => {
      throw new Error("rand should not be called while the countdown has not elapsed");
    };
    stepThunder(armed, FRAME_SEC, 0.5, rand);
    expect(armed.flash).toBeCloseTo(0.8 - FRAME_SEC * 5, 5);
    expect(armed.countdown).toBeCloseTo(5 - FRAME_SEC, 5);
  });

  it("storm passing (thunderRate drops to 0) lets a live flash decay to zero and stops scheduling", () => {
    const s = { flash: 0.9, countdown: 5, flashX: 0.5, flashY: 0.2 };
    const rand = () => {
      throw new Error("rand should not be called once thunderRate is 0");
    };
    for (let i = 0; i < 10; i++) {
      stepThunder(s, 0.5, 0, rand);
    }
    expect(s.flash).toBe(0);
    // countdown is left untouched (no longer meaningful once calm).
    expect(s.countdown).toBe(5);
  });

  it("mutates the state object in place instead of allocating a new one", () => {
    const s = initThunderState();
    const before = s;
    stepThunder(s, FRAME_SEC, 0.5, seqRand([0.1, 0.2, 0.3, 0.4]));
    expect(s).toBe(before); // same reference - zero allocation in the hot path
  });

  it("is deterministic: identical inputs produce identical state transitions", () => {
    const rand = seqRand([0.1, 0.2, 0.3, 0.4]);
    const rand2 = seqRand([0.1, 0.2, 0.3, 0.4]);
    const a = initThunderState();
    const b = initThunderState();
    stepThunder(a, FRAME_SEC, 1.2, rand);
    stepThunder(b, FRAME_SEC, 1.2, rand2);
    expect(a).toEqual(b);
  });
});
