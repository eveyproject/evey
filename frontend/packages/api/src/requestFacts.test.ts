import { describe, it, expect } from "vitest";
import { formatRating, formatReleaseDate } from "./requestFacts";

describe("formatReleaseDate", () => {
  it("renders the provider's date in the viewer's locale, on the same day", () => {
    expect(formatReleaseDate("2024-05-12", "en-US")).toBe("May 12, 2024");
    expect(formatReleaseDate("2024-05-12", "fr-FR")).toBe("12 mai 2024");
  });

  it("gives nothing for an empty or malformed date", () => {
    expect(formatReleaseDate("", "fr")).toBeUndefined();
    expect(formatReleaseDate(undefined, "fr")).toBeUndefined();
    expect(formatReleaseDate("2024", "fr")).toBeUndefined();
  });
});

describe("formatRating", () => {
  it("shows one decimal and hides an unrated or out-of-range score", () => {
    expect(formatRating(7.823)).toBe("7.8");
    expect(formatRating(10)).toBe("10.0");
    expect(formatRating(0)).toBeUndefined();
    expect(formatRating(11)).toBeUndefined();
    expect(formatRating(undefined)).toBeUndefined();
  });
});
