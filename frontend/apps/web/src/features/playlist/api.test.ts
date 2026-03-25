import React from "react";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
}));

async function getEveyFetch() {
  const { eveyFetch } = await import("@evey/api");
  return vi.mocked(eveyFetch);
}

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

describe("usePlaylistList", () => {
  beforeEach(() => vi.clearAllMocks());

  it("returns list of playlists", async () => {
    const fetch = await getEveyFetch();
    const playlists = [{ id: "p-1", name: "My Playlist" }];
    fetch.mockResolvedValueOnce({ playlists });

    const { usePlaylistList } = await import("./api");
    const { result } = renderHook(() => usePlaylistList(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(playlists);
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/api/v1/playlists", method: "GET" }),
    );
  });

  it("returns empty array when no playlists", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({});

    const { usePlaylistList } = await import("./api");
    const { result } = renderHook(() => usePlaylistList(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });
});

describe("usePlaylistDetail", () => {
  beforeEach(() => vi.clearAllMocks());

  it("fetches and maps playlist detail with items", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({
      playlist: {
        id: "p-1", name: "Watchlist", description: "My list", type: "manual",
        sortBy: "name", sortOrder: "asc", createdAt: "2024-01-01", updatedAt: "2024-01-02",
      },
      items: [{ mediaItemId: "m-1", sortOrder: 0, addedAt: "2024-01-01" }],
      media: [{ id: "m-1", title: "Movie A", type: "movie", year: 2024 }],
    });

    const { usePlaylistDetail } = await import("./api");
    const { result } = renderHook(() => usePlaylistDetail("p-1"), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.name).toBe("Watchlist");
    expect(result.current.data?.items?.[0].media_item_id).toBe("m-1");
    expect(result.current.data?.items?.[0].title).toBe("Movie A");
  });

  it("falls back to media array for smart playlists", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({
      playlist: {
        id: "p-2", name: "Smart", description: "", type: "smart",
        sortBy: "year", sortOrder: "desc", createdAt: "2024-01-01", updatedAt: "2024-01-01",
      },
      items: [],
      media: [{ id: "m-2", title: "Film B", type: "movie", year: 2023 }],
    });

    const { usePlaylistDetail } = await import("./api");
    const { result } = renderHook(() => usePlaylistDetail("p-2"), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.items?.[0].media_item_id).toBe("m-2");
    expect(result.current.data?.items?.[0].title).toBe("Film B");
  });

  it("is disabled when id is empty", async () => {
    const fetch = await getEveyFetch();

    const { usePlaylistDetail } = await import("./api");
    const { result } = renderHook(() => usePlaylistDetail(""), { wrapper: createWrapper() });

    expect(result.current.fetchStatus).toBe("idle");
    expect(fetch).not.toHaveBeenCalled();
  });
});

describe("useCreatePlaylist", () => {
  beforeEach(() => vi.clearAllMocks());

  it("calls POST and invalidates playlists query", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({ id: "new-1", name: "New" });

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(QueryClientProvider, { client: qc }, children);

    const { useCreatePlaylist } = await import("./api");
    const { result } = renderHook(() => useCreatePlaylist(), { wrapper });

    await act(async () => {
      result.current.mutate({ name: "New", type: "manual" });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/api/v1/playlists", method: "POST" }),
    );
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["playlists"] }),
    );
  });
});

describe("useUpdatePlaylist", () => {
  beforeEach(() => vi.clearAllMocks());

  it("calls PUT and invalidates playlists query", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce(undefined);

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(QueryClientProvider, { client: qc }, children);

    const { useUpdatePlaylist } = await import("./api");
    const { result } = renderHook(() => useUpdatePlaylist(), { wrapper });

    await act(async () => {
      result.current.mutate({ id: "p-1", name: "Updated" });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/api/v1/playlists/p-1", method: "PUT", data: { name: "Updated" } }),
    );
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["playlists"] }),
    );
  });
});

describe("useDeletePlaylist", () => {
  beforeEach(() => vi.clearAllMocks());

  it("calls DELETE and invalidates playlists query", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce(undefined);

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(QueryClientProvider, { client: qc }, children);

    const { useDeletePlaylist } = await import("./api");
    const { result } = renderHook(() => useDeletePlaylist(), { wrapper });

    await act(async () => {
      result.current.mutate("p-1");
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/api/v1/playlists/p-1", method: "DELETE" }),
    );
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["playlists"] }),
    );
  });
});

describe("useAddToPlaylist", () => {
  beforeEach(() => vi.clearAllMocks());

  it("calls POST items and invalidates playlist detail", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce(undefined);

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(QueryClientProvider, { client: qc }, children);

    const { useAddToPlaylist } = await import("./api");
    const { result } = renderHook(() => useAddToPlaylist(), { wrapper });

    await act(async () => {
      result.current.mutate({ playlistId: "p-1", mediaItemId: "m-1" });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/api/v1/playlists/p-1/items", method: "POST", data: { mediaItemId: "m-1" } }),
    );
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["playlists", "p-1"] }),
    );
  });
});

describe("useRemoveFromPlaylist", () => {
  beforeEach(() => vi.clearAllMocks());

  it("calls DELETE item and invalidates playlist detail", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce(undefined);

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(QueryClientProvider, { client: qc }, children);

    const { useRemoveFromPlaylist } = await import("./api");
    const { result } = renderHook(() => useRemoveFromPlaylist(), { wrapper });

    await act(async () => {
      result.current.mutate({ playlistId: "p-1", mediaItemId: "m-1" });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/api/v1/playlists/p-1/items/m-1", method: "DELETE" }),
    );
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["playlists", "p-1"] }),
    );
  });
});
