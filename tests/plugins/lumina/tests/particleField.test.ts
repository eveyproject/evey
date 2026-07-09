import { describe, it, expect } from "vitest";
import { createParticleField, particleBudgetForTier, MAX_PARTICLES } from "../src/domain/services/particleField";

describe("particleBudgetForTier", () => {
  it("is bounded and monotonic across tiers", () => {
    const low = particleBudgetForTier("low");
    const mid = particleBudgetForTier("mid");
    const high = particleBudgetForTier("high");
    expect(low).toBeGreaterThan(0);
    expect(low).toBeLessThan(mid);
    expect(mid).toBeLessThan(high);
    expect(high).toBe(MAX_PARTICLES);
  });
});

describe("createParticleField", () => {
  it("is deterministic: same count/seed produce the exact same field", () => {
    const a = createParticleField(20, 7.3);
    const b = createParticleField(20, 7.3);
    expect(a).toEqual(b);
  });

  it("different seeds produce different fields", () => {
    const a = createParticleField(20, 1);
    const b = createParticleField(20, 2);
    expect(a).not.toEqual(b);
  });

  it("returns exactly `count` particles for counts within budget", () => {
    expect(createParticleField(14, 1)).toHaveLength(14);
    expect(createParticleField(26, 1)).toHaveLength(26);
    expect(createParticleField(40, 1)).toHaveLength(40);
  });

  it("clamps the count to MAX_PARTICLES (bounded budget, never unbounded)", () => {
    expect(createParticleField(10_000, 1)).toHaveLength(MAX_PARTICLES);
  });

  it("clamps a negative count to zero instead of throwing", () => {
    expect(createParticleField(-5, 1)).toHaveLength(0);
  });

  it("every seed field is within its documented range", () => {
    const field = createParticleField(MAX_PARTICLES, 99.1);
    for (const p of field) {
      expect(p.baseX).toBeGreaterThanOrEqual(-1);
      expect(p.baseX).toBeLessThanOrEqual(1);
      expect(p.baseY).toBeGreaterThanOrEqual(-1);
      expect(p.baseY).toBeLessThanOrEqual(1);
      expect(p.depth).toBeGreaterThanOrEqual(0);
      expect(p.depth).toBeLessThanOrEqual(1);
      expect(p.size).toBeGreaterThanOrEqual(0.4);
      expect(p.size).toBeLessThanOrEqual(1.0);
      expect(p.phase).toBeGreaterThanOrEqual(0);
      expect(p.phase).toBeLessThanOrEqual(Math.PI * 2);
      expect(p.speed).toBeGreaterThanOrEqual(0.15);
      expect(p.speed).toBeLessThanOrEqual(0.5);
    }
  });
});
