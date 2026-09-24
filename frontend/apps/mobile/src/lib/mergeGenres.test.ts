import { describe, it, expect } from "vitest";
import { mergeGenres } from "./mergeGenres";

describe("mergeGenres", () => {
  it("merges several libraries without duplicating a shared genre", () => {
    expect(mergeGenres([["Action", "Drame"], ["Drame", "Comédie"]])).toEqual([
      "Action",
      "Comédie",
      "Drame",
    ]);
  });

  it("keeps the languages the libraries actually use, and invents none", () => {
    // The bug this replaces: a hardcoded English list merged in here showed
    // "Adventure" next to "Aventure" - the same genre twice - and the English
    // one matched nothing, because the filter compares the stored string.
    expect(mergeGenres([["Aventure", "Science-Fiction"]])).toEqual([
      "Aventure",
      "Science-Fiction",
    ]);
  });

  it("sorts with locale rules so accented genres land where a reader expects", () => {
    const out = mergeGenres([["Zeste", "Épouvante", "Action"]]);
    expect(out).toEqual(["Action", "Épouvante", "Zeste"]);
  });

  it("drops blank entries rather than rendering an unusable chip", () => {
    expect(mergeGenres([["Action", "", "   "]])).toEqual(["Action"]);
  });

  it("trims, so a padded duplicate does not become a second chip", () => {
    expect(mergeGenres([["Action", " Action "]])).toEqual(["Action"]);
  });

  it("survives a library that returned nothing", () => {
    expect(mergeGenres([undefined, ["Action"], []])).toEqual(["Action"]);
    expect(mergeGenres([])).toEqual([]);
  });
});

describe("mergeGenres - one entry per genre whatever its case", () => {
  // The phone's own copy listed "Science-Fiction" and "science-fiction" twice.
  it("keeps the first spelling of a genre met in another case", () => {
    expect(mergeGenres([["Science-Fiction"], ["science-fiction", "Drame"]])).toEqual(["Drame", "Science-Fiction"]);
  });
});
