import { describe, it, expect } from "vitest";
import { pickRepresentativeItems } from "../src/domain/services/representativeSelection";
import type { CinephileItem } from "../src/domain/model/media";

function makeItems(n: number): CinephileItem[] {
  return Array.from({ length: n }, (_, i) => ({ id: `mi-${i}`, title: `Title ${i}`, year: 2000, type: "movie" as const }));
}

describe("pickRepresentativeItems", () => {
  it("is deterministic for the same items/count/seed", () => {
    const items = makeItems(20);
    const a = pickRepresentativeItems(items, 4, 123);
    const b = pickRepresentativeItems(items, 4, 123);
    expect(a).toEqual(b);
  });

  it("returns exactly `count` items when there are enough", () => {
    const out = pickRepresentativeItems(makeItems(20), 4, 1);
    expect(out).toHaveLength(4);
  });

  it("returns every item (shuffled) when fewer than count are available", () => {
    const items = makeItems(2);
    const out = pickRepresentativeItems(items, 4, 1);
    expect(out).toHaveLength(2);
    expect(new Set(out.map((i) => i.id))).toEqual(new Set(items.map((i) => i.id)));
  });

  it("never returns duplicates", () => {
    const out = pickRepresentativeItems(makeItems(20), 5, 2);
    expect(new Set(out.map((i) => i.id)).size).toBe(out.length);
  });

  it("returns an empty array when count is 0", () => {
    expect(pickRepresentativeItems(makeItems(5), 0, 1)).toEqual([]);
  });

  it("returns an empty array for an empty input", () => {
    expect(pickRepresentativeItems([], 4, 1)).toEqual([]);
  });

  it("varies the selection across seeds", () => {
    const items = makeItems(50);
    const a = pickRepresentativeItems(items, 4, 1);
    const b = pickRepresentativeItems(items, 4, 2);
    expect(a).not.toEqual(b);
  });
});
