import { describe, it, expect } from "vitest";
import { resolveMediaLabel } from "./logMedia";
import type { V1MediaItem } from "@evey/api";

const items: Record<string, V1MediaItem> = {
  "mi-series": { id: "mi-series", type: "MEDIA_ITEM_TYPE_SERIES", title: "Some Series" } as V1MediaItem,
  "mi-season": { id: "mi-season", type: "MEDIA_ITEM_TYPE_SEASON", title: "Season 1", indexNumber: 1, parentId: "mi-series" } as V1MediaItem,
  "mi-ep": { id: "mi-ep", type: "MEDIA_ITEM_TYPE_EPISODE", title: "Pilot", indexNumber: 1, parentId: "mi-season" } as V1MediaItem,
  "mi-movie": { id: "mi-movie", type: "MEDIA_ITEM_TYPE_MOVIE", title: "Some Movie", year: 2016 } as V1MediaItem,
  "mi-orphan": { id: "mi-orphan", type: "MEDIA_ITEM_TYPE_EPISODE", title: "Some Episode", indexNumber: 3, parentId: "mi-gone" } as V1MediaItem,
};

const fetchItem = async (id: string): Promise<V1MediaItem> => {
  const it = items[id];
  if (!it) throw new Error("404");
  return it;
};

describe("resolveMediaLabel", () => {
  it("names an episode by its series, season and number, not just 'Pilot'", async () => {
    // "Pilot" is every series' first episode: the label must carry the series.
    await expect(resolveMediaLabel("mi-ep", fetchItem)).resolves.toEqual({ kind: "episode", label: "Some Series · S01E01 · Pilot" });
  });

  it("names a movie with its year", async () => {
    await expect(resolveMediaLabel("mi-movie", fetchItem)).resolves.toEqual({ kind: "movie", label: "Some Movie (2016)" });
  });

  it("names a season under its series", async () => {
    await expect(resolveMediaLabel("mi-season", fetchItem)).resolves.toEqual({ kind: "season", label: "Some Series S01" });
  });

  it("degrades to what it knows when a parent is gone", async () => {
    await expect(resolveMediaLabel("mi-orphan", fetchItem)).resolves.toEqual({ kind: "episode", label: "E03 · Some Episode" });
  });
});
