import { describe, it, expect } from "vitest";
import { decadeOf, formatDecadeRange, aggregateByDecade } from "../src/domain/services/decadeAggregation";
import type { CinephileItem } from "../src/domain/model/media";

function item(id: string, year: number): CinephileItem {
  return { id, title: `Title ${id}`, year, type: "movie" };
}

describe("decadeOf", () => {
  it("floors a year to its decade", () => {
    expect(decadeOf(1999)).toBe(1990);
    expect(decadeOf(1990)).toBe(1990);
    expect(decadeOf(2004)).toBe(2000);
    expect(decadeOf(2010)).toBe(2010);
  });
});

describe("formatDecadeRange", () => {
  it("formats a language-neutral numeric range", () => {
    expect(formatDecadeRange(1990)).toBe("1990-99");
    expect(formatDecadeRange(2000)).toBe("2000-09");
    expect(formatDecadeRange(2020)).toBe("2020-29");
  });
});

describe("aggregateByDecade", () => {
  it("groups items into decade buckets", () => {
    const items = [item("a", 1991), item("b", 1995), item("c", 2001)];
    const buckets = aggregateByDecade(items);
    expect(buckets).toHaveLength(2);
    const b1990 = buckets.find((b) => b.decade === 1990)!;
    expect(b1990.items.map((i) => i.id).sort()).toEqual(["a", "b"]);
  });

  it("sorts buckets chronologically ascending", () => {
    const items = [item("a", 2010), item("b", 1980), item("c", 2000)];
    const buckets = aggregateByDecade(items);
    expect(buckets.map((b) => b.decade)).toEqual([1980, 2000, 2010]);
  });

  it("skips items without a year", () => {
    const items = [item("a", 1990), { id: "b", title: "No year", type: "movie" as const }];
    const buckets = aggregateByDecade(items);
    const total = buckets.reduce((sum, b) => sum + b.items.length, 0);
    expect(total).toBe(1);
  });

  it("returns an empty array for an empty input", () => {
    expect(aggregateByDecade([])).toEqual([]);
  });
});
