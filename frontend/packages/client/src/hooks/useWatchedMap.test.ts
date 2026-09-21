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
