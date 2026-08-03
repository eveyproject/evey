import { describe, it, expect } from "vitest";
import { heroTitle } from "./heroTitle";

const SERIES = { id: "mi-series", title: "Special Ops: Lioness", type: "MEDIA_ITEM_TYPE_SERIES" };

describe("heroTitle", () => {
  it("leaves a film alone", () => {
    expect(heroTitle({ id: "mi-1", title: "Ava", type: "MEDIA_ITEM_TYPE_MOVIE" }, [])).toBe("Ava");
  });

  it("names the series in front of a season", () => {
    const season = { id: "mi-s3", title: "Season 3", type: "MEDIA_ITEM_TYPE_SEASON", parentId: "mi-series" };
    expect(heroTitle(season, [SERIES, season])).toBe("Special Ops: Lioness - Season 3");
  });

  // An episode hangs off its season, so naming it needs two hops - one only
  // would give "Season 3 - The Spider and the Fly", still nameless.
  it("walks past the season to name the series in front of an episode", () => {
    const season = { id: "mi-s3", title: "Season 3", type: "MEDIA_ITEM_TYPE_SEASON", parentId: "mi-series" };
    const ep = { id: "mi-e1", title: "The Spider and the Fly", type: "MEDIA_ITEM_TYPE_EPISODE", parentId: "mi-s3" };
    expect(heroTitle(ep, [SERIES, season, ep])).toBe("Special Ops: Lioness - The Spider and the Fly");
  });

  it("falls back to the season when the series is absent from the response", () => {
    const season = { id: "mi-s3", title: "Season 3", type: "MEDIA_ITEM_TYPE_SEASON", parentId: "mi-series" };
    const ep = { id: "mi-e1", title: "The Spider and the Fly", type: "MEDIA_ITEM_TYPE_EPISODE", parentId: "mi-s3" };
    expect(heroTitle(ep, [season, ep])).toBe("Season 3 - The Spider and the Fly");
  });

  it("does not hang on a parent chain that loops", () => {
    const a = { id: "mi-a", title: "A", type: "MEDIA_ITEM_TYPE_EPISODE", parentId: "mi-b" };
    const b = { id: "mi-b", title: "B", type: "MEDIA_ITEM_TYPE_SEASON", parentId: "mi-a" };
    expect(heroTitle(a, [a, b])).toBe("B - A");
  });

  // The parent is only usually in the same response; a missing one must leave
  // the plain title rather than a dangling separator.
  it("keeps the bare title when the parent is not in the response", () => {
    const season = { id: "mi-s3", title: "Season 3", type: "MEDIA_ITEM_TYPE_SEASON", parentId: "mi-elsewhere" };
    expect(heroTitle(season, [season])).toBe("Season 3");
  });

  it("keeps the bare title when the parent has no name", () => {
    const season = { id: "mi-s3", title: "Season 3", type: "MEDIA_ITEM_TYPE_SEASON", parentId: "mi-series" };
    expect(heroTitle(season, [{ id: "mi-series", title: "" }, season])).toBe("Season 3");
  });

  it("does not repeat a season named exactly like its series", () => {
    const season = { id: "mi-s1", title: "Special Ops: Lioness", type: "MEDIA_ITEM_TYPE_SEASON", parentId: "mi-series" };
    expect(heroTitle(season, [SERIES, season])).toBe("Special Ops: Lioness");
  });

  it("survives an item with no title at all", () => {
    expect(heroTitle({ id: "mi-x", type: "MEDIA_ITEM_TYPE_MOVIE" }, [])).toBe("");
  });
});
