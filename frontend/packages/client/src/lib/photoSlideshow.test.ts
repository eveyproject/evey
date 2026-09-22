import { describe, it, expect } from "vitest";
import {
  prevSlideshowIndex,
  nextSlideshowIndex,
  clampSlideshowIndex,
  nextAutoplayIndex,
  nextAutoplayIndexPaged,
} from "./photoSlideshow";

describe("prevSlideshowIndex / nextSlideshowIndex (manual D-pad nav)", () => {
  it("clamps at the first photo", () => {
    expect(prevSlideshowIndex(0, 5)).toBe(0);
  });

  it("clamps at the last photo", () => {
    expect(nextSlideshowIndex(4, 5)).toBe(4);
  });

  it("steps normally in the middle of the album", () => {
    expect(prevSlideshowIndex(2, 5)).toBe(1);
    expect(nextSlideshowIndex(2, 5)).toBe(3);
  });

  it("is a no-op on an empty album", () => {
    expect(prevSlideshowIndex(0, 0)).toBe(0);
    expect(nextSlideshowIndex(0, 0)).toBe(0);
  });
});

describe("clampSlideshowIndex", () => {
  it("clamps a negative index to 0", () => {
    expect(clampSlideshowIndex(-3, 5)).toBe(0);
  });

  it("clamps an out-of-range index to the last photo", () => {
    expect(clampSlideshowIndex(99, 5)).toBe(4);
  });

  it("resolves an empty album to 0", () => {
    expect(clampSlideshowIndex(2, 0)).toBe(0);
  });

  it("passes through an in-range index unchanged", () => {
    expect(clampSlideshowIndex(2, 5)).toBe(2);
  });
});

describe("nextAutoplayIndex", () => {
  it("loops from the last photo back to the first", () => {
    expect(nextAutoplayIndex(4, 5)).toBe(0);
  });

  it("steps normally otherwise", () => {
    expect(nextAutoplayIndex(1, 5)).toBe(2);
  });

  it("stays at 0 for an empty album", () => {
    expect(nextAutoplayIndex(0, 0)).toBe(0);
  });
});

describe("nextAutoplayIndexPaged", () => {
  // The slideshow pages its listing, so "how many photos are loaded" and "how
  // big the album is" are different numbers while pages are arriving. Wrapping
  // on the loaded count sent the viewer back to photo 1 at the end of every
  // page instead of continuing through the album.
  it("holds on the last loaded photo while more pages are coming", () => {
    expect(nextAutoplayIndexPaged(99, 100, true)).toBe(99);
  });

  it("wraps only once the album is fully loaded", () => {
    expect(nextAutoplayIndexPaged(99, 100, false)).toBe(0);
  });

  it("advances normally in the middle of the loaded set, page or no page", () => {
    expect(nextAutoplayIndexPaged(4, 100, true)).toBe(5);
    expect(nextAutoplayIndexPaged(4, 100, false)).toBe(5);
  });

  it("stays at 0 on an empty album", () => {
    expect(nextAutoplayIndexPaged(0, 0, true)).toBe(0);
    expect(nextAutoplayIndexPaged(3, 0, false)).toBe(0);
  });
});
