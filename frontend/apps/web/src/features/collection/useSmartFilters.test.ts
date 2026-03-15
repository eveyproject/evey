import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";

const mockDeleteMutate = vi.fn();

vi.mock("@/features/playlist/api", () => ({
  usePlaylistList: vi.fn(() => ({ data: undefined, isLoading: false })),
  useDeletePlaylist: vi.fn(() => ({ mutate: mockDeleteMutate })),
}));

vi.mock("./SmartCollectionDialog", () => ({
  playlistToSmartFilter: (pl: { id: string; name: string }) => ({
    id: pl.id,
    name: pl.name,
    rules: [],
    sortBy: "title",
    sortOrder: "asc",
  }),
}));

describe("useSmartFilters", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("returns empty filters when no playlists", async () => {
    const { useSmartFilters } = await import("./useSmartFilters");
    const { result } = renderHook(() => useSmartFilters());
    expect(result.current.filters).toEqual([]);
  });

  it("filters only smart playlists", async () => {
    const { usePlaylistList } = await import("@/features/playlist/api");
    vi.mocked(usePlaylistList).mockReturnValue({
      data: [
        { id: "pl1", name: "Favorites", type: "manual" } as never,
        { id: "pl2", name: "Action Movies", type: "smart" } as never,
        { id: "pl3", name: "New Releases", type: "smart" } as never,
      ],
      isLoading: false,
    } as ReturnType<typeof usePlaylistList>);
    const { useSmartFilters } = await import("./useSmartFilters");
    const { result } = renderHook(() => useSmartFilters());
    expect(result.current.filters).toHaveLength(2);
    expect(result.current.filters.map((f) => f.id)).toEqual(["pl2", "pl3"]);
  });

  it("propagates isLoading from usePlaylistList", async () => {
    const { usePlaylistList } = await import("@/features/playlist/api");
    vi.mocked(usePlaylistList).mockReturnValue({
      data: undefined,
      isLoading: true,
    } as ReturnType<typeof usePlaylistList>);
    const { useSmartFilters } = await import("./useSmartFilters");
    const { result } = renderHook(() => useSmartFilters());
    expect(result.current.isLoading).toBe(true);
  });

  it("remove calls useDeletePlaylist mutate with id", async () => {
    const { useSmartFilters } = await import("./useSmartFilters");
    const { result } = renderHook(() => useSmartFilters());
    result.current.remove("pl2");
    expect(mockDeleteMutate).toHaveBeenCalledWith("pl2");
  });

  it("save is a no-op (returns undefined)", async () => {
    const { useSmartFilters } = await import("./useSmartFilters");
    const { result } = renderHook(() => useSmartFilters());
    const ret = result.current.save({ id: "x", name: "X", rules: [], sortBy: "title", sortOrder: "asc" });
    expect(ret).toBeUndefined();
  });
});
