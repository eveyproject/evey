import { describe, it, expect } from "vitest";
import { genreTranslationKey, mergeGenres, genreHref } from "./genres";

describe("genreTranslationKey", () => {
  it("builds a media.genres.* key from a known genre", () => {
    expect(genreTranslationKey("Sci-Fi")).toBe("media.genres.scifi");
  });

  it("strips spaces and punctuation for a multi-word genre", () => {
    expect(genreTranslationKey("Action & Adventure")).toBe("media.genres.actionadventure");
  });

  it("still produces a key for a genre absent from media.genres.* (caller falls back to the raw string)", () => {
    expect(genreTranslationKey("Anime")).toBe("media.genres.anime");
  });
});

describe("mergeGenres", () => {
  it("dedupes case-insensitively across libraries, keeping the first-seen casing", () => {
    expect(mergeGenres([["Sci-Fi", "Drama"], ["sci-fi", "Comedy"]])).toEqual(["Comedy", "Drama", "Sci-Fi"]);
  });

  it("sorts the merged result", () => {
    expect(mergeGenres([["Western", "Action"]])).toEqual(["Action", "Western"]);
  });

  it("trims whitespace and drops empty entries", () => {
    expect(mergeGenres([[" Drama ", ""]])).toEqual(["Drama"]);
  });

  it("returns an empty array for no genres anywhere (every library empty or undefined)", () => {
    expect(mergeGenres([[], undefined, []])).toEqual([]);
  });

  it("returns an empty array for zero libraries", () => {
    expect(mergeGenres([])).toEqual([]);
  });
});

describe("genreHref", () => {
  it("encodes the genre into the dynamic route segment", () => {
    expect(genreHref("Sci-Fi")).toBe("/(auth)/genres/Sci-Fi");
  });

  it("encodes spaces and ampersands so the route segment stays a single path part", () => {
    expect(genreHref("Action & Adventure")).toBe("/(auth)/genres/Action%20%26%20Adventure");
  });
});
