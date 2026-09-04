import { describe, expect, it } from "vitest";
import { genreDotColor, genreHue } from "./genreColor";

describe("genreHue", () => {
  it("is stable across calls - a genre must not change colour on reload", () => {
    expect(genreHue("Action")).toBe(genreHue("Action"));
    expect(genreHue("Science-Fiction")).toBe(genreHue("Science-Fiction"));
  });

  it("ignores case and surrounding space", () => {
    expect(genreHue("  drame ")).toBe(genreHue("Drame"));
  });

  it("stays in range", () => {
    for (const g of ["Action", "Drame", "Comédie", "Western", "Mecha", "", "?"]) {
      const h = genreHue(g);
      expect(h).toBeGreaterThanOrEqual(0);
      expect(h).toBeLessThan(360);
    }
  });

  // Not a guarantee of the algorithm, but a smoke test that it spreads: if a
  // whole library's genres collapsed onto a couple of hues the feature would be
  // pointless.
  it("spreads a real library's genres over many hues", () => {
    const genres = [
      "Action", "Aventure", "Animation", "Comédie", "Crime", "Documentaire",
      "Drame", "Familial", "Fantastique", "Guerre", "Histoire", "Horreur",
      "Musique", "Mystère", "Romance", "Science-Fiction", "Thriller", "Western",
    ];
    const hues = new Set(genres.map((g) => Math.floor(genreHue(g) / 30)));
    expect(hues.size).toBeGreaterThanOrEqual(6);
  });
});

describe("genreDotColor", () => {
  it("gives the same genre the same colour", () => {
    expect(genreDotColor("Thriller")).toBe(genreDotColor("Thriller"));
  });

  it("gives different genres different colours", () => {
    expect(genreDotColor("Action")).not.toBe(genreDotColor("Romance"));
  });

  // The point of fixing saturation and lightness rather than hashing them too:
  // no genre may land on a near-white or near-black dot, on either theme.
  it("holds saturation and lightness constant for every genre", () => {
    for (const g of ["Action", "Drame", "Western", "Mecha", "Zzz", ""]) {
      expect(genreDotColor(g)).toMatch(/^hsl\(\d{1,3} 65% 55%\)$/);
    }
  });
});
