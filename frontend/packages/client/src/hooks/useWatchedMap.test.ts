import { renderHook } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";

// useWatchedMap is a pure useMemo over a generated query hook - mock the
// generated hook directly so there is no QueryClient dependency.
vi.mock("@evey/api", () => ({
  usePlaybackServiceGetWatchHistory: vi.fn(),
  WATCH_HISTORY_OVERLAY_LIMIT: 500,
}));

async function getWatchHistoryMock() {
  const { usePlaybackServiceGetWatchHistory } = await import("@evey/api");
  return vi.mocked(usePlaybackServiceGetWatchHistory);
}

describe("useWatchedMap", () => {
  it("includes items with played:true", async () => {
    const mock = await getWatchHistoryMock();
    mock.mockReturnValue({
      data: {
        items: [
          { mediaItemId: "m1", played: true },
          { mediaItemId: "m2", played: true },
        ],
      },
    } as ReturnType<typeof mock>);

    const { useWatchedMap } = await import("./useWatchedMap");
    const { result } = renderHook(() => useWatchedMap());

    expect(result.current.get("m1")).toBe(true);
    expect(result.current.get("m2")).toBe(true);
    expect(result.current.size).toBe(2);
  });

  it("excludes items with played:false", async () => {
    const mock = await getWatchHistoryMock();
    mock.mockReturnValue({
      data: {
        items: [
          { mediaItemId: "m1", played: false },
          { mediaItemId: "m2", played: true },
        ],
      },
    } as ReturnType<typeof mock>);

    const { useWatchedMap } = await import("./useWatchedMap");
    const { result } = renderHook(() => useWatchedMap());

    expect(result.current.has("m1")).toBe(false);
    expect(result.current.get("m2")).toBe(true);
    expect(result.current.size).toBe(1);
  });

  it("excludes items with no mediaItemId", async () => {
    const mock = await getWatchHistoryMock();
    mock.mockReturnValue({
      data: {
        items: [
          { mediaItemId: undefined, played: true },
          { mediaItemId: "m3", played: true },
        ],
      },
    } as ReturnType<typeof mock>);

    const { useWatchedMap } = await import("./useWatchedMap");
    const { result } = renderHook(() => useWatchedMap());

    expect(result.current.size).toBe(1);
    expect(result.current.get("m3")).toBe(true);
  });

  it("returns empty map when data is undefined", async () => {
    const mock = await getWatchHistoryMock();
    mock.mockReturnValue({ data: undefined } as ReturnType<typeof mock>);

    const { useWatchedMap } = await import("./useWatchedMap");
    const { result } = renderHook(() => useWatchedMap());

    expect(result.current.size).toBe(0);
  });

  it("returns empty map when items array is empty", async () => {
    const mock = await getWatchHistoryMock();
    mock.mockReturnValue({
      data: { items: [] },
    } as ReturnType<typeof mock>);

    const { useWatchedMap } = await import("./useWatchedMap");
    const { result } = renderHook(() => useWatchedMap());

    expect(result.current.size).toBe(0);
  });
});

describe("the watched mark on a title being watched again", () => {
  const rewatching = { mediaItemId: "m1", played: true, positionMs: "600000", durationMs: "3600000", percentComplete: 16.6 };

  it("is absent from the badge maps, with the progress kept", async () => {
    const { buildWatchProgressMaps } = await import("./useWatchedMap");
    const { watchedMap, percentMap } = buildWatchProgressMaps([rewatching, { mediaItemId: "m2", played: true }]);

    expect(watchedMap.get("m1")).toBeUndefined();
    expect(watchedMap.get("m2")).toBe(true);
    expect(percentMap.get("m1")).toBeCloseTo(16.6);
  });

  it("stays in the stored flag the lists filter by", async () => {
    const mock = await getWatchHistoryMock();
    mock.mockReturnValue({ data: { items: [rewatching] } } as ReturnType<typeof mock>);

    const { useWatchedMap } = await import("./useWatchedMap");
    const { result } = renderHook(() => useWatchedMap());

    expect(result.current.get("m1")).toBe(true);
  });
});

describe("the server's watched mark (user_data)", () => {
  it("wins over the overlay, which only answers for a title without it", async () => {
    const { itemPlayed, itemShowsWatched, filterUnwatched, itemPercent } = await import("./useWatchedMap");
    const legacy = new Map([["old", true], ["new-unwatched", true]]);
    // The overlay says watched, the server says not: the server is right.
    expect(itemPlayed({ id: "new-unwatched", userData: { played: false } }, legacy)).toBe(false);
    // A title marked by hand, missing from the overlay's window: still watched.
    expect(itemPlayed({ id: "marked", userData: { played: true } }, new Map())).toBe(true);
    // An old server sends no user_data: the overlay answers.
    expect(itemPlayed({ id: "old" }, legacy)).toBe(true);
    expect(itemPlayed({ id: "other" }, legacy)).toBe(false);
    expect(filterUnwatched([
      { id: "a", userData: { played: true } }, { id: "b", userData: { played: false } }, { id: "old" }, { id: "c" },
    ], legacy).map((i) => i.id)).toEqual(["b", "c"]);
    // A second viewing in progress shows its progress, not the mark.
    expect(itemShowsWatched({ id: "r", userData: { played: true, positionMs: "1800000", durationMs: "6000000" } })).toBe(false);
    expect(itemShowsWatched({ id: "r", userData: { played: true } })).toBe(true);
    expect(itemPercent({ id: "p", userData: { percentComplete: 142 } })).toBe(100);
  });
});
