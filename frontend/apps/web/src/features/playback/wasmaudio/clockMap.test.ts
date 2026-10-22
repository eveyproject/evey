import { describe, it, expect } from "vitest";
import { ClockMap, DRIFT_SOFT_RATE, DRIFT_SOFT_START_SEC, DRIFT_SOFT_STOP_SEC, driftCorrection } from "./clockMap";

describe("ClockMap", () => {
  it("maps both ways on one anchor", () => {
    const m = new ClockMap();
    m.reset(10, 700, 1);
    expect(m.ctxForMedia(706)).toBeCloseTo(16, 12);
    expect(m.mediaAtCtx(13)).toBeCloseTo(703, 12);
    expect(m.rateForMedia(800)).toBe(1);
  });

  it("bends with no gap and no overlap at the junction", () => {
    const m = new ClockMap();
    m.reset(10, 700, 1);
    m.bendFrom(720, 1 + DRIFT_SOFT_RATE);
    // A segment ending at 720 and the next one starting there meet on one instant.
    expect(m.ctxForMedia(720)).toBeCloseTo(30, 12);
    expect(m.rateForMedia(719.9)).toBe(1);
    expect(m.rateForMedia(720)).toBe(1 + DRIFT_SOFT_RATE);
    // 60 s of media past the bend take 60 ms less context time.
    expect(m.ctxForMedia(780) - m.ctxForMedia(720)).toBeCloseTo(60 / (1 + DRIFT_SOFT_RATE), 12);
    // The context reads the old rate before the bend, the new one after.
    expect(m.mediaAtCtx(20)).toBeCloseTo(710, 12);
    expect(m.mediaAtCtx(40)).toBeCloseTo(730 + 10 * DRIFT_SOFT_RATE, 9);
  });

  it("knows a bend the context has not reached, and forgets the ones it left", () => {
    const m = new ClockMap();
    m.reset(10, 700, 1);
    m.bendFrom(720, 1.001);
    expect(m.pendingAt(25)).toBe(true);
    expect(m.pendingAt(31)).toBe(false);
    m.prune(31);
    expect(m.ctxForMedia(730)).toBeCloseTo(30 + 10 / 1.001, 9);
    expect(m.mediaAtCtx(31)).toBeCloseTo(721.001, 9);
  });

  it("a later bend replaces whatever followed its instant", () => {
    const m = new ClockMap();
    m.reset(0, 0, 1);
    m.bendFrom(20, 1.001);
    m.bendFrom(10, 0.999);
    expect(m.rateForMedia(25)).toBe(0.999);
    expect(m.ctxForMedia(10)).toBeCloseTo(10, 12);
  });
});

describe("driftCorrection", () => {
  it("leaves a drift under the start threshold alone", () => {
    expect(driftCorrection(DRIFT_SOFT_START_SEC - 0.001, 0)).toBe(0);
    expect(driftCorrection(-(DRIFT_SOFT_START_SEC - 0.001), 0)).toBe(0);
  });

  it("speeds a sound that is behind the picture up, and slows one ahead down", () => {
    expect(driftCorrection(0.05, 0)).toBe(1); // picture ahead of the sound
    expect(driftCorrection(-0.05, 0)).toBe(-1);
  });

  it("holds the correction down to the stop threshold, then lets go", () => {
    expect(driftCorrection(0.02, 1)).toBe(1);
    expect(driftCorrection(DRIFT_SOFT_STOP_SEC - 0.001, 1)).toBe(0);
    expect(driftCorrection(-0.005, 1)).toBe(0); // overshot: stop, do not swing
  });

  it("ignores a drift it cannot read", () => {
    expect(driftCorrection(NaN, 1)).toBe(1);
  });
});
