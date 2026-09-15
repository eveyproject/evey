import { describe, it, expect } from "vitest";
import { shuffleArray } from "./shuffleArray";

describe("shuffleArray", () => {
  it("returns a permutation and leaves the input untouched", () => {
    const input = [1, 2, 3, 4, 5, 6, 7, 8];
    const frozen = [...input];
    const out = shuffleArray(input);
    expect(input).toEqual(frozen);
    expect(out).not.toBe(input);
    expect([...out].sort((a, b) => a - b)).toEqual(frozen);
  });

  it("handles the degenerate sizes", () => {
    expect(shuffleArray([])).toEqual([]);
    expect(shuffleArray(["only"])).toEqual(["only"]);
  });

  it("actually moves things around", () => {
    const input = Array.from({ length: 30 }, (_, i) => i);
    const same = Array.from({ length: 20 }, () => shuffleArray(input)).every((out) => out.every((v, i) => v === i));
    expect(same).toBe(false);
  });
});
