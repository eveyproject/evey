import { describe, expect, it } from "vitest";
import { selectSearchResultsView } from "./searchResultsView";

const base = { searchKind: "media" as const, isError: false, activeLoading: false, hasQuery: true, musicItemsCount: 0, mediaItemsCount: 0 };

describe("selectSearchResultsView", () => {
  it("an error on the active side wins over an empty result", () => {
    expect(selectSearchResultsView({ ...base, isError: true })).toBe("error");
    expect(selectSearchResultsView({ ...base, searchKind: "music", isError: true, musicItemsCount: 3 })).toBe("error");
  });

  it("asks for a query before anything else", () => {
    expect(selectSearchResultsView({ ...base, hasQuery: false, isError: true, activeLoading: true })).toBe("prompt");
  });

  it("each scope answers for its own results", () => {
    expect(selectSearchResultsView({ ...base, mediaItemsCount: 2 })).toBe("mediaResults");
    expect(selectSearchResultsView({ ...base })).toBe("mediaEmpty");
    expect(selectSearchResultsView({ ...base, searchKind: "music", musicItemsCount: 2 })).toBe("musicResults");
    expect(selectSearchResultsView({ ...base, searchKind: "music" })).toBe("musicEmpty");
  });

  it("loading only shows once something was typed", () => {
    expect(selectSearchResultsView({ ...base, activeLoading: true })).toBe("loading");
    expect(selectSearchResultsView({ ...base, activeLoading: true, hasQuery: false })).toBe("prompt");
  });
});
