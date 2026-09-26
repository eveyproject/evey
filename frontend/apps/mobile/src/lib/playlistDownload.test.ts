import { describe, it, expect, vi } from "vitest";
import type { V1MediaItem } from "@evey/api";
import { downloadPlaylistItems } from "./playlistDownload";

const items: Record<string, V1MediaItem> = {
  movie: { id: "movie", type: "MEDIA_ITEM_TYPE_MOVIE", title: "Some Movie", containerFormat: "matroska", officialRating: "PG",
    streams: [{ type: "MEDIA_STREAM_TYPE_AUDIO", codec: "ac3" }, { type: "MEDIA_STREAM_TYPE_VIDEO", codec: "h264" }] } as V1MediaItem,
  ep: { id: "ep", type: "MEDIA_ITEM_TYPE_EPISODE", title: "Some Episode", seriesTitle: "Some Series", indexNumber: 3 } as V1MediaItem,
  track: { id: "track", type: "MEDIA_ITEM_TYPE_TRACK", title: "Some Track" } as V1MediaItem,
  broken: { id: "broken", type: "MEDIA_ITEM_TYPE_MOVIE", title: "Some Other Movie" } as V1MediaItem,
};

function deps(onDisk: string[] = []) {
  return {
    getItem: vi.fn(async (id: string) => items[id]),
    isDownloaded: vi.fn(async (id: string) => onDisk.includes(id)),
    start: vi.fn(async (id: string) => { if (id === "broken") throw new Error("503"); return "path"; }),
    authContext: vi.fn((item: V1MediaItem) => ({ officialRating: item.officialRating })),
  };
}

describe("downloadPlaylistItems", () => {
  it("downloads each title with what the offline player needs", async () => {
    const d = deps();
    const r = await downloadPlaylistItems(["movie", "ep"], d);
    expect(r).toEqual({ started: 2, skipped: 0, failed: 0 });
    expect(d.start).toHaveBeenNthCalledWith(1, "movie", "Some Movie", expect.objectContaining({
      kind: "video", formatHint: "matroska", audioCodecs: ["ac3"], officialRating: "PG",
    }));
    expect(d.start).toHaveBeenNthCalledWith(2, "ep", "Some Episode", expect.objectContaining({
      display: { seriesTitle: "Some Series", episodeNumber: 3 },
    }));
  });

  it("skips what is already on the device and what is not a video, and one failure does not stop the rest", async () => {
    const d = deps(["movie"]);
    const r = await downloadPlaylistItems(["movie", "track", "broken", "ep"], d);
    expect(r).toEqual({ started: 1, skipped: 2, failed: 1 });
    expect(d.getItem).not.toHaveBeenCalledWith("movie");
    expect(d.start).toHaveBeenLastCalledWith("ep", "Some Episode", expect.anything());
  });
});
