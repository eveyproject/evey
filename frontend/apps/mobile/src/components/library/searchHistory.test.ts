import { describe, it, expect } from "vitest";
import { MAX_SEARCH_HISTORY, pushSearchHistory, removeSearchHistoryEntry } from "./searchHistory";

describe("MAX_SEARCH_HISTORY", () => {
  it("matches web's cap (useRecentSearches.ts MAX = 8)", () => {
    expect(MAX_SEARCH_HISTORY).toBe(8);
  });
});

describe("pushSearchHistory", () => {
  it("adds a new query to the front", () => {
    expect(pushSearchHistory(["b", "a"], "c")).toEqual(["c", "b", "a"]);
  });

  it("trims whitespace", () => {
    expect(pushSearchHistory([], "  matrix  ")).toEqual(["matrix"]);
  });

  it("ignores an empty/whitespace-only query", () => {
    expect(pushSearchHistory(["a"], "   ")).toEqual(["a"]);
  });

  it("de-duplicates an existing entry, moving it to the front", () => {
    expect(pushSearchHistory(["a", "b", "c"], "b")).toEqual(["b", "a", "c"]);
  });

  it("clamps to MAX_SEARCH_HISTORY entries", () => {
    const history = Array.from({ length: MAX_SEARCH_HISTORY }, (_, i) => `q${i}`);
    const result = pushSearchHistory(history, "new");
    expect(result).toHaveLength(MAX_SEARCH_HISTORY);
    expect(result[0]).toBe("new");
    expect(result).not.toContain(`q${MAX_SEARCH_HISTORY - 1}`);
  });

  it("honors a custom max override", () => {
    expect(pushSearchHistory(["a", "b"], "c", 2)).toEqual(["c", "a"]);
  });
});

describe("removeSearchHistoryEntry", () => {
  it("removes only the matching entry", () => {
    expect(removeSearchHistoryEntry(["a", "b", "c"], "b")).toEqual(["a", "c"]);
  });

  it("is a no-op when the entry isn't present", () => {
    expect(removeSearchHistoryEntry(["a", "b"], "z")).toEqual(["a", "b"]);
  });

  it("returns an empty array when removing the last entry", () => {
    expect(removeSearchHistoryEntry(["a"], "a")).toEqual([]);
  });
});
