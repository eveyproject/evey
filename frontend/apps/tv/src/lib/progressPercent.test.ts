import { describe, it, expect } from "vitest";
import { progressPercent } from "./progressPercent";

describe("progressPercent", () => {
  it("passes an ordinary share through", () => {
    expect(progressPercent(42)).toBe(42);
    expect(progressPercent(0)).toBe(0);
    expect(progressPercent(100)).toBe(100);
  });

  // A position past the declared duration is a real shape here, and it painted
  // a bar wider than its container.
  it("never exceeds a full bar", () => {
    expect(progressPercent(104)).toBe(100);
    expect(progressPercent(1000)).toBe(100);
  });

  it("never goes negative", () => {
    expect(progressPercent(-5)).toBe(0);
  });

  it("treats a missing or unusable value as nothing watched", () => {
    expect(progressPercent(undefined)).toBe(0);
    expect(progressPercent(null)).toBe(0);
    expect(progressPercent(Number.NaN)).toBe(0);
  });
});
