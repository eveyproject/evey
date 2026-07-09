import { describe, it, expect } from "vitest";
import { selectWallPosters } from "../src/domain/services/posterSelection";
import type { MediaSummary } from "../src/domain/model/media";

function makeItems(n: number): MediaSummary[] {
  return Array.from({ length: n }, (_, i) => ({
    id: `mi-${i}`,
    title: `Movie ${i}`,
    posterUrl: `https://host/poster-${i}.jpg`,
  }));
}

describe("selectWallPosters", () => {
  it("is deterministic for the same items/count/seed", () => {
    const items = makeItems(50);
    const a = selectWallPosters(items, 24, 123);
    const b = selectWallPosters(items, 24, 123);
    expect(a).toEqual(b);
  });

  it("returns exactly `count` posters when the library is large enough", () => {
    const items = makeItems(50);
    const out = selectWallPosters(items, 24, 1);
    expect(out).toHaveLength(24);
  });

  it("never returns duplicates", () => {
    const items = makeItems(50);
    const out = selectWallPosters(items, 24, 5);
    expect(new Set(out.map((i) => i.id)).size).toBe(out.length);
  });

  it("only returns items that are a subset of the input", () => {
    const items = makeItems(50);
    const out = selectWallPosters(items, 24, 5);
    const inputIds = new Set(items.map((i) => i.id));
    expect(out.every((i) => inputIds.has(i.id))).toBe(true);
  });

  it("returns every eligible item (shuffled) when the library is smaller than count", () => {
    const items = makeItems(5);
    const out = selectWallPosters(items, 24, 5);
    expect(out).toHaveLength(5);
    expect(new Set(out.map((i) => i.id))).toEqual(new Set(items.map((i) => i.id)));
  });

  it("drops items without a poster url", () => {
    const items = makeItems(10);
    items[0] = { ...items[0], posterUrl: "" };
    const out = selectWallPosters(items, 10, 1);
    expect(out.find((i) => i.id === "mi-0")).toBeUndefined();
  });

  it("drops duplicate ids from the input", () => {
    const items = makeItems(3);
    const withDupe = [...items, { ...items[0] }];
    const out = selectWallPosters(withDupe, 10, 1);
    expect(out).toHaveLength(3);
  });

  it("returns an empty array when count is 0", () => {
    expect(selectWallPosters(makeItems(10), 0, 1)).toEqual([]);
  });

  it("returns an empty array when there is nothing eligible", () => {
    expect(selectWallPosters([], 24, 1)).toEqual([]);
  });

  it("varies the selection across seeds on a large library", () => {
    const items = makeItems(200);
    const a = selectWallPosters(items, 24, 1);
    const b = selectWallPosters(items, 24, 2);
    expect(a).not.toEqual(b);
  });
});
