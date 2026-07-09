import { describe, it, expect } from "vitest";
import { placeholderColorForId } from "../src/domain/services/placeholderPalette";

describe("placeholderColorForId", () => {
  it("is deterministic for the same id", () => {
    expect(placeholderColorForId("mi-abc")).toEqual(placeholderColorForId("mi-abc"));
  });

  it("returns an RGB triplet in [0,1]", () => {
    const [r, g, b] = placeholderColorForId("mi-xyz");
    for (const c of [r, g, b]) {
      expect(c).toBeGreaterThanOrEqual(0);
      expect(c).toBeLessThanOrEqual(1);
    }
  });

  it("varies across different ids", () => {
    const colors = new Set(
      Array.from({ length: 20 }, (_, i) => JSON.stringify(placeholderColorForId(`mi-${i}`))),
    );
    expect(colors.size).toBeGreaterThan(1);
  });

  it("does not throw on an empty id", () => {
    expect(() => placeholderColorForId("")).not.toThrow();
  });
});
