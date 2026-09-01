import { describe, it, expect } from "vitest";
import { toggleFeed, moveFeed, changedFeeds } from "./feedConfigEdits";

const rows = [
  { feedType: "continue", enabled: true, displayOrder: 0 },
  { feedType: "recent", enabled: true, displayOrder: 1 },
  { feedType: "popular", enabled: false, displayOrder: 2 },
];

describe("feed config edits", () => {
  it("a toggle changes exactly one row, and only that row is persisted", () => {
    const next = toggleFeed(rows, 2);
    expect(next[2].enabled).toBe(true);
    expect(changedFeeds(rows, next).map((c) => c.feedType)).toEqual(["popular"]);
  });

  it("a move swaps two neighbours, renumbers, and persists exactly those two", () => {
    const next = moveFeed(rows, 2, -1);
    expect(next.map((c) => c.feedType)).toEqual(["continue", "popular", "recent"]);
    expect(next.map((c) => c.displayOrder)).toEqual([0, 1, 2]);
    expect(changedFeeds(rows, next).map((c) => c.feedType).sort()).toEqual(["popular", "recent"]);
  });

  it("a move off the ends is a no-op that persists nothing", () => {
    expect(moveFeed(rows, 0, -1)).toBe(rows);
    expect(changedFeeds(rows, moveFeed(rows, 2, 1))).toEqual([]);
  });

  it("an unchanged list persists nothing", () => {
    expect(changedFeeds(rows, rows.map((c) => ({ ...c })))).toEqual([]);
  });
});
