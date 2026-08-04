import { describe, it, expect, vi, beforeEach } from "vitest";

const eveyFetchMock = vi.fn();
vi.mock("@evey/api", () => ({ eveyFetch: (...args: unknown[]) => eveyFetchMock(...args) }));

import { usePlaylistList, usePlaylistDetail, useAddToPlaylist, useUpdatePlaylist } from "./playlists";
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

// These used to mock a wrapper the endpoint never sent ({playlist, items,
// media}) and asserted a merge that could not happen: the real GET answers with
// the playlist flat, and the members come from their own endpoint. The old
// shape made the tests green while opening a playlist threw on device.
describe("usePlaylistDetail", () => {
  function mockTwoCalls(playlist: Record<string, unknown>, items: unknown[]) {
    eveyFetchMock.mockImplementation(({ url }: { url: string }) =>
      Promise.resolve(url.endsWith("/items") ? { items } : playlist),
    );
  }

  it("reads the flat playlist and its members from their own endpoint", async () => {
    mockTwoCalls(
      {
        id: "p1", name: "Mix", description: "desc", type: "manual",
        sortBy: "name", sortOrder: "asc", createdAt: "2025-01-01", updatedAt: "2025-01-01",
      },
      [{ mediaItemId: "m1", sortOrder: 0, addedAt: "2025-01-01", title: "Movie A", type: "movie", year: 2020 }],
    );

    const { result } = renderHook(() => usePlaylistDetail("p1"), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(eveyFetchMock).toHaveBeenCalledWith({ url: "/api/v1/playlists/p1", method: "GET" });
    expect(eveyFetchMock).toHaveBeenCalledWith({ url: "/api/v1/playlists/p1/items", method: "GET" });
    expect(result.current.data?.name).toBe("Mix");
    expect(result.current.data?.items?.[0]).toMatchObject({
      media_item_id: "m1",
      title: "Movie A",
      type: "movie",
      year: 2020,
    });
  });

  it("gives an empty member list when the playlist holds none", async () => {
    mockTwoCalls(
      {
        id: "p2", name: "Auto", description: "", type: "smart",
        sortBy: "date", sortOrder: "desc", createdAt: "2025-01-01", updatedAt: "2025-01-01",
      },
      [],
    );

    const { result } = renderHook(() => usePlaylistDetail("p2"), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.id).toBe("p2");
    expect(result.current.data?.items).toEqual([]);
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

describe("useUpdatePlaylist", () => {
  // The proto declares UpdatePlaylist as PATCH (proto/evey/v1/playlist.proto);
  // the client used to send PUT, which grpc-gateway's generated mux 404s.
  it("sends PATCH (not PUT) to /api/v1/playlists/:id", async () => {
    eveyFetchMock.mockResolvedValue(undefined);

    const { result } = renderHook(() => useUpdatePlaylist(), { wrapper: createWrapper() });

    await act(async () => {
      result.current.mutate({ id: "p1", name: "Renamed" });
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/playlists/p1",
      method: "PATCH",
      data: { name: "Renamed" },
    });
  });
});
