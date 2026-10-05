import { describe, it, expect, vi } from "vitest";
import type { V1MediaItem } from "@evey/api";
import { seriesDownloadTargets } from "./seriesDownloadTargets";

vi.mock("../components/media/mediaFile", () => ({
  hasMediaFile: (item: V1MediaItem) => !!item.filePath,
}));

const ep = (id: string, indexNumber: number, filePath?: string): V1MediaItem => ({ id, indexNumber, filePath });

describe("seriesDownloadTargets", () => {
  it("collects every season's episodes in order, each with its season number", async () => {
    const seasons: V1MediaItem[] = [{ id: "s2", indexNumber: 2 }, { id: "s1", indexNumber: 1 }];
    const episodes: Record<string, V1MediaItem[]> = {
      s1: [ep("e1-2", 2, "/a"), ep("e1-1", 1, "/b")],
      s2: [ep("e2-1", 1, "/c"), ep("e2-2", 2)],
    };
    const targets = await seriesDownloadTargets(seasons, async (id) => episodes[id] ?? []);
    expect(targets.map((t) => [t.episode.id, t.seasonNumber])).toEqual([
      ["e1-1", 1],
      ["e1-2", 1],
      ["e2-1", 2],
    ]);
  });
});
