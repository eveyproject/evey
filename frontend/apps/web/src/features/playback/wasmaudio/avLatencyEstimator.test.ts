import { describe, it, expect } from "vitest";
import { AVLatencyEstimator } from "./avLatencyEstimator";

describe("AVLatencyEstimator - compensation A/V mesurée, pas devinée", () => {
  it("reste muet tant que la mesure n'est pas mûre (le repli statique s'applique)", () => {
    const e = new AVLatencyEstimator();
    expect(e.estimate).toBeNull();
    for (let i = 0; i < 9; i++) e.update(0.5);
    expect(e.estimate).toBeNull(); // 9 < minimum
    e.update(0.5);
    expect(e.estimate).not.toBeNull();
  });

  it("converge vers la latence réelle du pipeline (cas Mi TV ~0,55 s)", () => {
    const e = new AVLatencyEstimator();
    for (let i = 0; i < 200; i++) e.update(0.55 + (i % 2 === 0 ? 0.02 : -0.02));
    expect(e.estimate!).toBeGreaterThan(0.5);
    expect(e.estimate!).toBeLessThan(0.6);
  });

  it("jette les échantillons aberrants (stall, mesure cassée) sans polluer l'EMA", () => {
    const e = new AVLatencyEstimator();
    for (let i = 0; i < 50; i++) e.update(0.1);
    e.update(15); // stall
    e.update(-3); // mesure cassée
    e.update(NaN);
    expect(e.estimate!).toBeCloseTo(0.1, 2);
  });

  it("borne l'estimation : jamais négative, jamais au-delà de 1,5 s", () => {
    const low = new AVLatencyEstimator();
    for (let i = 0; i < 50; i++) low.update(-0.1); // jitter légèrement négatif accepté
    expect(low.estimate!).toBe(0);
    const high = new AVLatencyEstimator();
    for (let i = 0; i < 400; i++) high.update(1.9);
    expect(high.estimate!).toBe(1.5);
  });
});
