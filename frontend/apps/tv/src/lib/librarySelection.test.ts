import { describe, it, expect, vi, afterEach } from "vitest";
import type { V1MediaItem } from "@evey/api";
import {
  isPlayableMediaType,
  canPlayAllForSelection,
  buildPlayAllQueue,
  pickRandomItem,
} from "./librarySelection";

function item(id: string, type: V1MediaItem["type"]): V1MediaItem {
  return { id, type };
}

describe("isPlayableMediaType", () => {
  it("treats movies and episodes as directly playable", () => {
    expect(isPlayableMediaType("MEDIA_ITEM_TYPE_MOVIE")).toBe(true);
    expect(isPlayableMediaType("MEDIA_ITEM_TYPE_EPISODE")).toBe(true);
  });

  it("treats series and seasons as groupings with nothing to play", () => {
    expect(isPlayableMediaType("MEDIA_ITEM_TYPE_SERIES")).toBe(false);
    expect(isPlayableMediaType("MEDIA_ITEM_TYPE_SEASON")).toBe(false);
  });

  it("is false when the type is missing", () => {
    expect(isPlayableMediaType(undefined)).toBe(false);
  });
});

describe("canPlayAllForSelection", () => {
  it("is always true for a movies library, regardless of the type filter", () => {
    expect(canPlayAllForSelection(false, "")).toBe(true);
    expect(canPlayAllForSelection(false, "MEDIA_ITEM_TYPE_MOVIE")).toBe(true);
  });

  // The default shows-library view lists series, which have no stream of
  // their own - a "play all" control there would always be a dead click.
  it("is false on a shows library with no type filter (series rows)", () => {
    expect(canPlayAllForSelection(true, "")).toBe(false);
  });

  it("is false on a shows library filtered to seasons (still not playable)", () => {
    expect(canPlayAllForSelection(true, "MEDIA_ITEM_TYPE_SEASON")).toBe(false);
  });

  it("is true on a shows library once filtered to episodes", () => {
    expect(canPlayAllForSelection(true, "MEDIA_ITEM_TYPE_EPISODE")).toBe(true);
  });
});

describe("buildPlayAllQueue", () => {
  it("returns an empty queue for an empty selection", () => {
    expect(buildPlayAllQueue([])).toEqual([]);
  });

  it("builds a queue of one for a single playable item", () => {
    expect(buildPlayAllQueue([item("mi-1", "MEDIA_ITEM_TYPE_MOVIE")])).toEqual(["mi-1"]);
  });

  it("keeps only playable items, in the order they were given (the caller's sort/filter)", () => {
    const items = [
      item("s-1", "MEDIA_ITEM_TYPE_SERIES"),
      item("e-1", "MEDIA_ITEM_TYPE_EPISODE"),
      item("season-1", "MEDIA_ITEM_TYPE_SEASON"),
      item("e-2", "MEDIA_ITEM_TYPE_EPISODE"),
    ];
    expect(buildPlayAllQueue(items)).toEqual(["e-1", "e-2"]);
  });

  it("drops items missing an id rather than pushing an unusable route", () => {
    expect(buildPlayAllQueue([{ type: "MEDIA_ITEM_TYPE_MOVIE" }])).toEqual([]);
  });
});

describe("pickRandomItem", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("returns undefined for an empty selection instead of crashing", () => {
    expect(pickRandomItem([])).toBeUndefined();
  });

  it("returns the only element of a single-item selection", () => {
    expect(pickRandomItem(["only"])).toBe("only");
  });

  it("picks strictly within the given selection", () => {
    vi.spyOn(Math, "random").mockReturnValue(0.999);
    const items = ["a", "b", "c"];
    expect(pickRandomItem(items)).toBe("c");
  });
});
