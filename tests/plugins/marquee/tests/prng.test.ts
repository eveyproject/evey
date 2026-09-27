import { describe, it, expect } from "vitest";
import { mulberry32, hashString, shuffleInPlace } from "../src/domain/services/prng";

describe("mulberry32", () => {
  it("is deterministic for a given seed", () => {
    const a = mulberry32(42);
    const b = mulberry32(42);
    const seqA = [a(), a(), a()];
    const seqB = [b(), b(), b()];
    expect(seqA).toEqual(seqB);
  });

  it("produces values in [0, 1)", () => {
    const rng = mulberry32(7);
    for (let i = 0; i < 50; i++) {
      const v = rng();
      expect(v).toBeGreaterThanOrEqual(0);
      expect(v).toBeLessThan(1);
    }
  });

  it("differs across seeds", () => {
    const a = mulberry32(1)();
    const b = mulberry32(2)();
    expect(a).not.toBe(b);
  });
});

describe("hashString", () => {
  it("is deterministic", () => {
    expect(hashString("mi-abc123")).toBe(hashString("mi-abc123"));
  });

  it("returns an unsigned 32-bit integer", () => {
    const h = hashString("Some Movie");
    expect(h).toBeGreaterThanOrEqual(0);
    expect(Number.isInteger(h)).toBe(true);
  });

  it("handles the empty string", () => {
    expect(() => hashString("")).not.toThrow();
  });
});

describe("shuffleInPlace", () => {
  it("is deterministic for a given rng seed", () => {
    const a = shuffleInPlace([1, 2, 3, 4, 5], mulberry32(9));
    const b = shuffleInPlace([1, 2, 3, 4, 5], mulberry32(9));
    expect(a).toEqual(b);
  });

  it("preserves the multiset of elements", () => {
    const input = [1, 2, 3, 4, 5];
    const out = shuffleInPlace(input.slice(), mulberry32(1));
    expect(out.slice().sort()).toEqual(input.slice().sort());
  });

  it("returns the same array reference it was given", () => {
    const arr = [1, 2, 3];
    expect(shuffleInPlace(arr, mulberry32(1))).toBe(arr);
  });
});
