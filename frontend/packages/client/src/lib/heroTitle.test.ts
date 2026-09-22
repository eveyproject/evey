import { describe, it, expect } from "vitest";
import { heroTitle } from "./heroTitle";

const SERIES = { id: "mi-series", title: "Some Ops: Some Series", type: "MEDIA_ITEM_TYPE_SERIES" };

// Stands in for i18next: proves the label went through media.season rather
// than through whatever the scan froze in the row.
const t = (key: string, options?: Record<string, unknown>) =>
  key === "media.season" ? `Saison ${options?.number}` : key;

describe("heroTitle", () => {
  it("leaves a film alone", () => {
    expect(heroTitle({ id: "mi-1", title: "Ava", type: "MEDIA_ITEM_TYPE_MOVIE" }, [], t)).toBe("Ava");
  });

  it("names the series in front of a season", () => {
    const season = { id: "mi-s3", title: "Season 3", indexNumber: 3, type: "MEDIA_ITEM_TYPE_SEASON", parentId: "mi-series" };
    expect(heroTitle(season, [SERIES, season], t)).toBe("Some Ops: Some Series - Saison 3");
  });

  // No index_number (a Meilisearch hit carries none): the stored title stays
  // rather than a hero reading "Saison undefined".
  it("falls back to the stored title when a season has no number", () => {
    const season = { id: "mi-s3", title: "Season 3", type: "MEDIA_ITEM_TYPE_SEASON", parentId: "mi-series" };
    expect(heroTitle(season, [SERIES, season], t)).toBe("Some Ops: Some Series - Season 3");
  });

  // An episode hangs off its season, so naming it needs two hops - one only
  // would give "Season 3 - Some Episode", still nameless.
  it("walks past the season to name the series in front of an episode", () => {
    const season = { id: "mi-s3", title: "Season 3", type: "MEDIA_ITEM_TYPE_SEASON", parentId: "mi-series" };
    const ep = { id: "mi-e1", title: "Some Episode", type: "MEDIA_ITEM_TYPE_EPISODE", parentId: "mi-s3" };
    expect(heroTitle(ep, [SERIES, season, ep], t)).toBe("Some Ops: Some Series - Some Episode");
  });

  it("falls back to the season when the series is absent from the response", () => {
    const season = { id: "mi-s3", title: "Season 3", indexNumber: 3, type: "MEDIA_ITEM_TYPE_SEASON", parentId: "mi-series" };
    const ep = { id: "mi-e1", title: "Some Episode", type: "MEDIA_ITEM_TYPE_EPISODE", parentId: "mi-s3" };
    expect(heroTitle(ep, [season, ep], t)).toBe("Saison 3 - Some Episode");
  });

  it("does not hang on a parent chain that loops", () => {
    const a = { id: "mi-a", title: "A", type: "MEDIA_ITEM_TYPE_EPISODE", parentId: "mi-b" };
    const b = { id: "mi-b", title: "B", type: "MEDIA_ITEM_TYPE_SEASON", parentId: "mi-a" };
    expect(heroTitle(a, [a, b], t)).toBe("B - A");
  });

  // The parent is only usually in the same response; a missing one must leave
  // the plain title rather than a dangling separator.
  it("keeps the bare title when the parent is not in the response", () => {
    const season = { id: "mi-s3", title: "Season 3", type: "MEDIA_ITEM_TYPE_SEASON", parentId: "mi-elsewhere" };
    expect(heroTitle(season, [season], t)).toBe("Season 3");
  });

  it("keeps the bare title when the parent has no name", () => {
    const season = { id: "mi-s3", title: "Season 3", type: "MEDIA_ITEM_TYPE_SEASON", parentId: "mi-series" };
    expect(heroTitle(season, [{ id: "mi-series", title: "" }, season], t)).toBe("Season 3");
  });

  it("does not repeat a season named exactly like its series", () => {
    const season = { id: "mi-s1", title: "Some Ops: Some Series", type: "MEDIA_ITEM_TYPE_SEASON", parentId: "mi-series" };
    expect(heroTitle(season, [SERIES, season], t)).toBe("Some Ops: Some Series");
  });

  it("survives an item with no title at all", () => {
    expect(heroTitle({ id: "mi-x", type: "MEDIA_ITEM_TYPE_MOVIE" }, [], t)).toBe("");
  });
});
