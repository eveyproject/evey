import { describe, it, expect } from "vitest";
import { prevLightboxIndex, nextLightboxIndex, rotateClockwise, rotateCounterClockwise } from "./lightboxNav";

describe("prevLightboxIndex", () => {
  it("stays on the first photo instead of going negative", () => {
    expect(prevLightboxIndex(0, 3)).toBe(0);
  });

  it("steps back one photo from the middle", () => {
    expect(prevLightboxIndex(1, 3)).toBe(0);
  });

  it("steps back from the last photo", () => {
    expect(prevLightboxIndex(2, 3)).toBe(1);
  });

  it("returns the index unchanged for an empty list", () => {
    expect(prevLightboxIndex(0, 0)).toBe(0);
  });
});

describe("nextLightboxIndex", () => {
  it("stays on the last photo instead of overshooting", () => {
    expect(nextLightboxIndex(2, 3)).toBe(2);
  });

  it("steps forward one photo from the first", () => {
    expect(nextLightboxIndex(0, 3)).toBe(1);
  });

  it("steps forward from the middle", () => {
    expect(nextLightboxIndex(1, 3)).toBe(2);
  });

  it("returns the index unchanged for an empty list", () => {
    expect(nextLightboxIndex(0, 0)).toBe(0);
  });
});

describe("rotateClockwise", () => {
  it("steps by 90 degrees", () => {
    expect(rotateClockwise(0)).toBe(90);
    expect(rotateClockwise(90)).toBe(180);
    expect(rotateClockwise(270)).toBe(0);
  });
});

describe("rotateCounterClockwise", () => {
  it("steps back by 90 degrees, wrapping through 0", () => {
    expect(rotateCounterClockwise(0)).toBe(270);
    expect(rotateCounterClockwise(90)).toBe(0);
    expect(rotateCounterClockwise(270)).toBe(180);
  });
});
