import { describe, it, expect } from "vitest";
import { filterWatchable } from "../src/domain/services/mediaFilter";
import type { CinephileItem } from "../src/domain/model/media";

function item(overrides: Partial<CinephileItem> = {}): CinephileItem {
  return { id: "mi-1", title: "Some Movie", year: 1999, type: "movie", ...overrides };
}

describe("filterWatchable", () => {
  it("keeps movies and series with a valid year", () => {
    const out = filterWatchable([item({ type: "movie" }), item({ id: "mi-2", type: "series" })]);
    expect(out).toHaveLength(2);
  });

  it("drops other media kinds (episodes, albums, photos, ...)", () => {
    const out = filterWatchable([item({ type: "other" })]);
    expect(out).toHaveLength(0);
  });

  it("drops items without a year", () => {
    const out = filterWatchable([item({ year: undefined })]);
    expect(out).toHaveLength(0);
  });

  it("drops items with an out-of-range year", () => {
    const out = filterWatchable([item({ year: 1500 }), item({ id: "mi-2", year: 3000 })]);
    expect(out).toHaveLength(0);
  });

  it("drops items without an id or title", () => {
    const out = filterWatchable([item({ id: "" }), item({ id: "mi-2", title: "" })]);
    expect(out).toHaveLength(0);
  });

  it("drops duplicate ids, keeping the first occurrence", () => {
    const out = filterWatchable([item({ title: "First" }), item({ title: "Second" })]);
    expect(out).toHaveLength(1);
    expect(out[0].title).toBe("First");
  });

  it("returns an empty array for an empty input", () => {
    expect(filterWatchable([])).toEqual([]);
  });

  it("preserves item order for kept items", () => {
    const out = filterWatchable([
      item({ id: "mi-1", year: 2001 }),
      item({ id: "mi-2", year: 1990 }),
      item({ id: "mi-3", year: 2010 }),
    ]);
    expect(out.map((i) => i.id)).toEqual(["mi-1", "mi-2", "mi-3"]);
  });
});
