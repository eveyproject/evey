import { describe, it, expect } from "vitest";
import { AVLatencyEstimator } from "./avLatencyEstimator";

describe("AVLatencyEstimator - measured A/V compensation, not guessed", () => {
  it("stays null until the measurement is mature (the static fallback applies)", () => {
    const e = new AVLatencyEstimator();
    expect(e.estimate).toBeNull();
    for (let i = 0; i < 9; i++) e.update(0.5);
    expect(e.estimate).toBeNull(); // 9 < minimum
    e.update(0.5);
    expect(e.estimate).not.toBeNull();
  });

  it("converges to the pipeline's real latency (Mi TV case ~0.55s)", () => {
    const e = new AVLatencyEstimator();
    for (let i = 0; i < 200; i++) e.update(0.55 + (i % 2 === 0 ? 0.02 : -0.02));
    expect(e.estimate!).toBeGreaterThan(0.5);
    expect(e.estimate!).toBeLessThan(0.6);
  });

  it("discards outlier samples (stall, broken measurement) without polluting the EMA", () => {
    const e = new AVLatencyEstimator();
    for (let i = 0; i < 50; i++) e.update(0.1);
    e.update(15); // stall
    e.update(-3); // beyond any real sink
    e.update(NaN);
    expect(e.estimate!).toBeCloseTo(0.1, 2);
  });

  it("leads the audio when the sink is slower than the picture (Bluetooth speaker, ~0.9s)", () => {
    // sample = video display lag - audio output latency: 0.05 - 0.95
    const e = new AVLatencyEstimator();
    for (let i = 0; i < 400; i++) e.update(-0.9);
    expect(e.estimate!).toBeCloseTo(-0.9, 2);
  });

  it("bounds the estimate to 1.5s either way", () => {
    const low = new AVLatencyEstimator();
    for (let i = 0; i < 50; i++) low.update(-0.1);
    expect(low.estimate!).toBeCloseTo(-0.1, 2);
    const high = new AVLatencyEstimator();
    for (let i = 0; i < 400; i++) high.update(1.9);
    expect(high.estimate!).toBe(1.5);
    const tooLow = new AVLatencyEstimator();
    for (let i = 0; i < 50; i++) tooLow.update(-3); // beyond any real sink: discarded
    expect(tooLow.estimate).toBeNull();
  });
});
