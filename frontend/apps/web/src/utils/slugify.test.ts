import { describe, it, expect } from "vitest";
import { slugify } from "./slugify";

describe("slugify", () => {
  it("lowercases and replaces spaces with hyphens", () => {
    expect(slugify("My Movies")).toBe("my-movies");
  });

  it("handles TV Shows style names", () => {
    expect(slugify("TV Shows")).toBe("tv-shows");
  });

  it("strips accented characters", () => {
    expect(slugify("Mes Films Preferes")).toBe("mes-films-preferes");
    expect(slugify("Series")).toBe("series");
  });

  it("strips special characters", () => {
    expect(slugify("Movies & TV!")).toBe("movies-tv");
  });

  it("collapses multiple hyphens", () => {
    expect(slugify("a   b---c")).toBe("a-b-c");
  });

  it("trims leading/trailing hyphens", () => {
    expect(slugify("--hello--")).toBe("hello");
  });

  it("handles empty string", () => {
    expect(slugify("")).toBe("");
  });

  it("handles unicode accents", () => {
    expect(slugify("cafe")).toBe("cafe");
  });
});
