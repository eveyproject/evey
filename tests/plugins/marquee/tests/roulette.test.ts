import { describe, it, expect } from "vitest";
import { planRouletteSpin } from "../src/domain/services/roulette";

describe("planRouletteSpin", () => {
  it("is deterministic for the same seed", () => {
    const a = planRouletteSpin(24, 42);
    const b = planRouletteSpin(24, 42);
    expect(a).toEqual(b);
  });

  it("lands the last step on the winnerIndex", () => {
    const spin = planRouletteSpin(24, 7);
    expect(spin.steps[spin.steps.length - 1]?.index).toBe(spin.winnerIndex);
  });

  it("keeps every step index within [0, candidateCount)", () => {
    const spin = planRouletteSpin(9, 3);
    expect(spin.steps.every((s) => s.index >= 0 && s.index < 9)).toBe(true);
  });

  it("has strictly increasing timestamps", () => {
    const spin = planRouletteSpin(24, 11);
    for (let i = 1; i < spin.steps.length; i++) {
      expect(spin.steps[i].atMs).toBeGreaterThan(spin.steps[i - 1].atMs);
    }
  });

  it("decelerates: later gaps between steps are bigger than earlier ones", () => {
    const spin = planRouletteSpin(24, 11);
    const gap = (i: number) => spin.steps[i].atMs - spin.steps[i - 1].atMs;
    const earlyGap = gap(1);
    const lateGap = gap(spin.steps.length - 1);
    expect(lateGap).toBeGreaterThan(earlyGap);
  });

  it("totalDurationMs matches the last step's timestamp", () => {
    const spin = planRouletteSpin(24, 11);
    expect(spin.totalDurationMs).toBeGreaterThanOrEqual(spin.steps[spin.steps.length - 1].atMs);
  });

  it("varies the winner across seeds", () => {
    const winners = new Set<number>();
    for (let seed = 0; seed < 20; seed++) winners.add(planRouletteSpin(24, seed).winnerIndex);
    expect(winners.size).toBeGreaterThan(1);
  });

  it("handles a single candidate", () => {
    const spin = planRouletteSpin(1, 1);
    expect(spin.winnerIndex).toBe(0);
    expect(spin.steps.every((s) => s.index === 0)).toBe(true);
  });

  it("returns an empty spin for zero candidates", () => {
    const spin = planRouletteSpin(0, 1);
    expect(spin.winnerIndex).toBe(-1);
    expect(spin.steps).toEqual([]);
    expect(spin.totalDurationMs).toBe(0);
  });
});
