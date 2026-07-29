import { describe, it, expect } from "vitest";
import { photoSlideshowHref } from "./photoRoutes";

describe("photoSlideshowHref", () => {
  it("carries libraryId and startIndex", () => {
    expect(photoSlideshowHref("lib1", 3)).toBe(
      "/(auth)/photos/slideshow?libraryId=lib1&startIndex=3",
    );
  });

  it("carries album when set", () => {
    expect(photoSlideshowHref("lib1", 0, "Vacation")).toBe(
      "/(auth)/photos/slideshow?libraryId=lib1&startIndex=0&album=Vacation",
    );
  });
});
