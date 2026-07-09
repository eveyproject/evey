import { describe, it, expect } from "vitest";
import { computeTasteProfile } from "../src/domain/services/tasteProfile";
import type { CinephileItem } from "../src/domain/model/media";

function item(id: string, year: number, type: "movie" | "series" = "movie"): CinephileItem {
  return { id, title: `Title ${id}`, year, type };
}

describe("computeTasteProfile", () => {
  it("counts movies and series separately", () => {
    const profile = computeTasteProfile([item("a", 2000, "movie"), item("b", 2001, "series"), item("c", 2002, "movie")]);
    expect(profile.movieCount).toBe(2);
    expect(profile.seriesCount).toBe(1);
    expect(profile.totalCount).toBe(3);
  });

  it("finds the dominant decade and its share", () => {
    const items = [item("a", 1991), item("b", 1995), item("c", 1999), item("d", 2010)];
    const profile = computeTasteProfile(items);
    expect(profile.dominantDecade).toBe(1990);
    expect(profile.dominantDecadeShare).toBeCloseTo(0.75, 5);
  });

  it("computes the oldest and newest year", () => {
    const items = [item("a", 2005), item("b", 1988), item("c", 2020)];
    const profile = computeTasteProfile(items);
    expect(profile.oldestYear).toBe(1988);
    expect(profile.newestYear).toBe(2020);
  });

  it("handles an empty library without throwing", () => {
    const profile = computeTasteProfile([]);
    expect(profile.totalCount).toBe(0);
    expect(profile.dominantDecade).toBeNull();
    expect(profile.dominantDecadeShare).toBe(0);
    expect(profile.oldestYear).toBeNull();
    expect(profile.newestYear).toBeNull();
  });

  it("is deterministic for the same input", () => {
    const items = [item("a", 1990), item("b", 2001), item("c", 2001)];
    expect(computeTasteProfile(items)).toEqual(computeTasteProfile(items));
  });
});
