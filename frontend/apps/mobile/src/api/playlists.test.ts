import { describe, it, expect, vi, beforeEach } from "vitest";

const eveyFetchMock = vi.fn();
vi.mock("@evey/api", () => ({ eveyFetch: (...args: unknown[]) => eveyFetchMock(...args) }));

import { usePlaylistList, usePlaylistDetail, useAddToPlaylist } from "./playlists";
import { renderHook, waitFor, act } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

beforeEach(() => { eveyFetchMock.mockReset(); });

describe("usePlaylistList", () => {
  it("fetches /api/v1/playlists and returns playlists array", async () => {
    const playlists = [{ id: "p1", name: "Favorites" }];
    eveyFetchMock.mockResolvedValue({ playlists });

    const { result } = renderHook(() => usePlaylistList(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/playlists",
      method: "GET",
    });
    expect(result.current.data).toEqual(playlists);
  });

  it("returns empty array when playlists is undefined", async () => {
    eveyFetchMock.mockResolvedValue({});

    const { result } = renderHook(() => usePlaylistList(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual([]);
  });
});

describe("usePlaylistDetail", () => {
  it("fetches playlist by id and merges media into items", async () => {
    eveyFetchMock.mockResolvedValue({
      playlist: {
        id: "p1", name: "Mix", description: "desc", type: "manual",
        sortBy: "name", sortOrder: "asc", createdAt: "2025-01-01", updatedAt: "2025-01-01",
      },
      items: [{ mediaItemId: "m1", sortOrder: 0, addedAt: "2025-01-01" }],
      media: [{ id: "m1", title: "Movie A", type: "movie", year: 2020 }],
    });

    const { result } = renderHook(() => usePlaylistDetail("p1"), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/playlists/p1",
      method: "GET",
    });
    expect(result.current.data?.items?.[0]).toMatchObject({
      media_item_id: "m1",
      title: "Movie A",
      type: "movie",
      year: 2020,
    });
  });

  it("falls back to media array when items is empty", async () => {
    eveyFetchMock.mockResolvedValue({
      playlist: {
        id: "p2", name: "Auto", description: "", type: "smart",
        sortBy: "date", sortOrder: "desc", createdAt: "2025-01-01", updatedAt: "2025-01-01",
      },
      items: [],
      media: [
        { id: "m1", title: "A", type: "movie" },
        { id: "m2", title: "B", type: "series", year: 2021 },
      ],
    });

    const { result } = renderHook(() => usePlaylistDetail("p2"), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.items).toHaveLength(2);
    expect(result.current.data?.items?.[0].media_item_id).toBe("m1");
    expect(result.current.data?.items?.[1].sort_order).toBe(1);
  });

  it("is disabled when id is empty", () => {
    const { result } = renderHook(() => usePlaylistDetail(""), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe("idle");
  });
});

describe("useAddToPlaylist", () => {
  it("posts to /api/v1/playlists/:id/items", async () => {
    eveyFetchMock.mockResolvedValue(undefined);

    const { result } = renderHook(() => useAddToPlaylist(), { wrapper: createWrapper() });

    await act(async () => {
      result.current.mutate({ playlistId: "p1", mediaItemId: "m1" });
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/playlists/p1/items",
      method: "POST",
      data: { mediaItemId: "m1" },
    });
  });
});
