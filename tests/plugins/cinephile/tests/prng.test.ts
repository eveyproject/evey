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
    for (let i = 0; i < 100; i++) {
      const v = rng();
      expect(v).toBeGreaterThanOrEqual(0);
      expect(v).toBeLessThan(1);
    }
  });

  it("varies output across seeds", () => {
    const a = mulberry32(1)();
    const b = mulberry32(2)();
    expect(a).not.toBe(b);
  });
});

describe("hashString", () => {
  it("is deterministic for the same string", () => {
    expect(hashString("decade-1990")).toBe(hashString("decade-1990"));
  });

  it("differs across distinct strings (no trivial collision)", () => {
    expect(hashString("decade-1990")).not.toBe(hashString("decade-2000"));
  });

  it("returns a non-negative uint32", () => {
    const h = hashString("other");
    expect(h).toBeGreaterThanOrEqual(0);
    expect(Number.isInteger(h)).toBe(true);
  });
});

describe("shuffleInPlace", () => {
  it("is deterministic given the same rng sequence", () => {
    const a = shuffleInPlace([1, 2, 3, 4, 5], mulberry32(9));
    const b = shuffleInPlace([1, 2, 3, 4, 5], mulberry32(9));
    expect(a).toEqual(b);
  });

  it("preserves the same set of elements", () => {
    const out = shuffleInPlace([1, 2, 3, 4, 5], mulberry32(3));
    expect(new Set(out)).toEqual(new Set([1, 2, 3, 4, 5]));
  });

  it("mutates and returns the same array reference", () => {
    const arr = [1, 2, 3];
    const out = shuffleInPlace(arr, mulberry32(1));
    expect(out).toBe(arr);
  });
});
