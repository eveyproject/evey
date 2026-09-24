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
  // The server's own shape (enum type, camelCase), measured against the dev
  // server: the screens filter on "manual", so the list must translate it.
  it("fetches /api/v1/playlists and speaks the screens' type", async () => {
    eveyFetchMock.mockResolvedValue({
      playlists: [
        { id: "p1", name: "Favorites", type: "PLAYLIST_TYPE_MANUAL", sortBy: "sort_order", sortOrder: "asc", createdAt: "c", updatedAt: "u" },
        { id: "p2", name: "Rules", type: "PLAYLIST_TYPE_SMART" },
      ],
    });

    const { result } = renderHook(() => usePlaylistList(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/playlists",
      method: "GET",
    });
    expect(result.current.data?.map((p) => [p.id, p.type])).toEqual([["p1", "manual"], ["p2", "smart"]]);
    expect(result.current.data?.[0]).toMatchObject({ name: "Favorites", sort_by: "sort_order", sort_order: "asc", created_at: "c" });
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

describe("useCreatePlaylist", () => {
  // Measured on the dev server: type "manual" is refused (HTTP 400, invalid
  // enum), and the created playlist comes back flat, not under "playlist".
  it("sends the API's enum and returns the new id", async () => {
    const { useCreatePlaylist } = await import("./playlists");
    eveyFetchMock.mockResolvedValue({ id: "pl-9", name: "Soirée", type: "PLAYLIST_TYPE_MANUAL" });
    const { result } = renderHook(() => useCreatePlaylist(), { wrapper: createWrapper() });
    let created: { id: string } | undefined;
    await act(async () => { created = await result.current.mutateAsync({ name: "Soirée", type: "manual" }); });
    expect(eveyFetchMock).toHaveBeenCalledWith(expect.objectContaining({ url: "/api/v1/playlists", method: "POST", data: { name: "Soirée", type: "PLAYLIST_TYPE_MANUAL" } }));
    expect(created).toEqual({ id: "pl-9" });
  });
});
