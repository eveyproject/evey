import { describe, it, expect } from "vitest";
import { averageAlbumProgress } from "./albumDownloadProgress";

describe("averageAlbumProgress", () => {
  it("returns 0 for an empty track list", () => {
    expect(averageAlbumProgress([])).toBe(0);
  });

  it("averages progress across finished and in-flight tracks", () => {
    expect(averageAlbumProgress([1, 0.5, 0])).toBeCloseTo(0.5);
  });

  it("treats NaN (unknown total size) as 0 rather than poisoning the average", () => {
    expect(averageAlbumProgress([1, NaN])).toBe(0.5);
  });

  it("returns 1 when every track finished", () => {
    expect(averageAlbumProgress([1, 1, 1])).toBe(1);
  });
});
