// The playlists of every client (web, phone, both televisions) since
// 2026-09-24; this suite came from the web's own layer.
import React from "react";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, it, expect, vi, beforeEach } from "vitest";

const mockList = vi.fn(() => ({ data: { playlists: [] } }));
const mockGet = vi.fn(() => ({ data: undefined, isLoading: false, isSuccess: true }));
const mockGetItems = vi.fn(() => ({ data: { items: [] }, isLoading: false, isSuccess: true }));
const createFn = vi.fn();
const updateFn = vi.fn();
const deleteFn = vi.fn();
const addItemFn = vi.fn();
const removeItemFn = vi.fn();
const eveyFetchFn = vi.fn();

function sel<T>(raw: { data?: T; [k: string]: unknown }, opts: { query?: { select?: (d: T) => unknown; enabled?: boolean } }) {
  const s = opts?.query?.select;
  const enabled = opts?.query?.enabled !== false;
  return { ...raw, data: enabled && raw?.data !== undefined && s ? s(raw.data) : (enabled ? raw?.data : undefined), fetchStatus: enabled ? "idle" : "idle" };
}

vi.mock("@evey/api", () => ({
  eveyFetch: (...a: unknown[]) => eveyFetchFn(...a),
  usePlaylistServiceListPlaylists: (_p: unknown, opts: { query?: { select?: (d: unknown) => unknown } }) => sel(mockList(), opts),
  usePlaylistServiceGetPlaylist: (_id: string, _p: unknown, opts: { query?: { select?: (d: unknown) => unknown; enabled?: boolean } }) => sel(mockGet(), opts),
  usePlaylistServiceGetPlaylistItems: (_id: string, _p: unknown, opts: { query?: { select?: (d: unknown) => unknown; enabled?: boolean } }) => sel(mockGetItems(), opts),
  playlistServiceCreatePlaylist: (body: unknown) => createFn(body),
  playlistServiceUpdatePlaylist: (id: string, body: unknown) => updateFn(id, body),
  playlistServiceDeletePlaylist: (id: string) => deleteFn(id),
  playlistServiceAddPlaylistItem: (id: string, body: unknown) => addItemFn(id, body),
  playlistServiceRemovePlaylistItem: (id: string, mid: string) => removeItemFn(id, mid),
  getPlaylistServiceListPlaylistsQueryKey: () => ["playlists"],
  getPlaylistServiceGetPlaylistQueryKey: (id: string) => ["playlist", id],
  getPlaylistServiceGetPlaylistItemsQueryKey: (id: string) => ["playlist-items", id],
  V1PlaylistType: { PLAYLIST_TYPE_UNSPECIFIED: "PLAYLIST_TYPE_UNSPECIFIED", PLAYLIST_TYPE_MANUAL: "PLAYLIST_TYPE_MANUAL", PLAYLIST_TYPE_SMART: "PLAYLIST_TYPE_SMART" },
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return { qc, wrapper: ({ children }: { children: React.ReactNode }) => React.createElement(QueryClientProvider, { client: qc }, children) };
}

describe("usePlaylistList", () => {
  beforeEach(() => vi.clearAllMocks());

  it("returns list of playlists", async () => {
    mockList.mockReturnValue({ data: { playlists: [{ id: "p-1", name: "My Playlist", type: "PLAYLIST_TYPE_MANUAL" }] } });
    const { usePlaylistList } = await import("./playlists");
    const { wrapper } = createWrapper();
    const { result } = renderHook(() => usePlaylistList(), { wrapper });
    expect(result.current.data?.[0]?.id).toBe("p-1");
  });

  it("returns empty array when no playlists", async () => {
    mockList.mockReturnValue({ data: {} });
    const { usePlaylistList } = await import("./playlists");
    const { wrapper } = createWrapper();
    const { result } = renderHook(() => usePlaylistList(), { wrapper });
    expect(result.current.data).toEqual([]);
  });
});

describe("useCreatePlaylist", () => {
  beforeEach(() => vi.clearAllMocks());

  it("calls create and invalidates playlists query", async () => {
    createFn.mockResolvedValue({ id: "new-1", name: "New" });
    const { useCreatePlaylist } = await import("./playlists");
    const { qc, wrapper } = createWrapper();
    const spy = vi.spyOn(qc, "invalidateQueries");
    const { result } = renderHook(() => useCreatePlaylist(), { wrapper });
    await act(async () => { result.current.mutate({ name: "New", type: "manual" }); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(createFn).toHaveBeenCalled();
    expect(spy).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["playlists"] }));
  });
});

describe("useUpdatePlaylist", () => {
  beforeEach(() => vi.clearAllMocks());

  it("calls update and invalidates playlists query", async () => {
    updateFn.mockResolvedValue(undefined);
    const { useUpdatePlaylist } = await import("./playlists");
    const { qc, wrapper } = createWrapper();
    const spy = vi.spyOn(qc, "invalidateQueries");
    const { result } = renderHook(() => useUpdatePlaylist(), { wrapper });
    await act(async () => { result.current.mutate({ id: "p-1", name: "Updated" }); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(updateFn).toHaveBeenCalledWith("p-1", expect.objectContaining({ name: "Updated" }));
    expect(spy).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["playlists"] }));
  });
});

describe("useDeletePlaylist", () => {
  beforeEach(() => vi.clearAllMocks());

  it("calls delete and invalidates playlists query", async () => {
    deleteFn.mockResolvedValue(undefined);
    const { useDeletePlaylist } = await import("./playlists");
    const { qc, wrapper } = createWrapper();
    const spy = vi.spyOn(qc, "invalidateQueries");
    const { result } = renderHook(() => useDeletePlaylist(), { wrapper });
    await act(async () => { result.current.mutate("p-1"); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(deleteFn).toHaveBeenCalledWith("p-1");
    expect(spy).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["playlists"] }));
  });
});

describe("useAddToPlaylist", () => {
  beforeEach(() => vi.clearAllMocks());

  it("calls add item and invalidates items", async () => {
    addItemFn.mockResolvedValue(undefined);
    const { useAddToPlaylist } = await import("./playlists");
    const { qc, wrapper } = createWrapper();
    const spy = vi.spyOn(qc, "invalidateQueries");
    const { result } = renderHook(() => useAddToPlaylist(), { wrapper });
    await act(async () => { result.current.mutate({ playlistId: "p-1", mediaItemId: "m-1" }); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(addItemFn).toHaveBeenCalledWith("p-1", { mediaItemId: "m-1" });
    expect(spy).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["playlist-items", "p-1"] }));
  });
});

describe("useRemoveFromPlaylist", () => {
  beforeEach(() => vi.clearAllMocks());

  it("calls remove item and invalidates items", async () => {
    removeItemFn.mockResolvedValue(undefined);
    const { useRemoveFromPlaylist } = await import("./playlists");
    const { qc, wrapper } = createWrapper();
    const spy = vi.spyOn(qc, "invalidateQueries");
    const { result } = renderHook(() => useRemoveFromPlaylist(), { wrapper });
    await act(async () => { result.current.mutate({ playlistId: "p-1", mediaItemId: "m-1" }); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(removeItemFn).toHaveBeenCalledWith("p-1", "m-1");
    expect(spy).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["playlist-items", "p-1"] }));
  });
});

// What the phone's former copy got wrong, measured against the dev server.
describe("the API's own shapes", () => {
  beforeEach(() => vi.clearAllMocks());

  it("speaks the screens' type: the list's enum becomes manual or smart", async () => {
    mockList.mockReturnValue({ data: { playlists: [{ id: "p1", name: "A", type: "PLAYLIST_TYPE_MANUAL" }, { id: "p2", name: "B", type: "PLAYLIST_TYPE_SMART" }] } });
    const { usePlaylistList } = await import("./playlists");
    const { wrapper } = createWrapper();
    const { result } = renderHook(() => usePlaylistList(), { wrapper });
    expect(result.current.data?.map((p) => p.type)).toEqual(["manual", "smart"]);
  });

  it("creates with the enum (the API refuses 'manual') and returns the flat id", async () => {
    createFn.mockResolvedValue({ id: "pl-9", name: "Soirée", type: "PLAYLIST_TYPE_MANUAL" });
    const { useCreatePlaylist } = await import("./playlists");
    const { wrapper } = createWrapper();
    const { result } = renderHook(() => useCreatePlaylist(), { wrapper });
    let created: { id: string } | undefined;
    await act(async () => { created = await result.current.mutateAsync({ name: "Soirée", type: "manual" }); });
    expect(createFn).toHaveBeenCalledWith(expect.objectContaining({ name: "Soirée", type: "PLAYLIST_TYPE_MANUAL" }));
    expect(created).toEqual({ id: "pl-9" });
  });

  it("tells a music track from a video when adding", async () => {
    addItemFn.mockResolvedValue(undefined);
    const { useAddToPlaylist } = await import("./playlists");
    const { wrapper } = createWrapper();
    const { result } = renderHook(() => useAddToPlaylist(), { wrapper });
    await act(async () => { await result.current.mutateAsync({ playlistId: "p-1", mediaItemId: "t-1", mediaType: "music" }); });
    expect(addItemFn).toHaveBeenCalledWith("p-1", { mediaItemId: "t-1", mediaType: "music" });
  });

  it("writes a hand-set order to the reorder endpoint", async () => {
    eveyFetchFn.mockResolvedValue(undefined);
    const { useReorderPlaylistItems } = await import("./playlists");
    const { wrapper } = createWrapper();
    const { result } = renderHook(() => useReorderPlaylistItems(), { wrapper });
    await act(async () => { await result.current.mutateAsync({ playlistId: "p-1", items: [{ mediaItemId: "a", sortOrder: 0 }] }); });
    expect(eveyFetchFn).toHaveBeenCalledWith({ url: "/api/v1/playlists/p-1/items/reorder", method: "PUT", data: { items: [{ mediaItemId: "a", sortOrder: 0 }] } });
  });
});

// A caller that toasts its own failure (the smart rules editor) asks the
// global mutation net to stay quiet; every other caller keeps the net's toast.
describe("useUpdatePlaylist error ownership", () => {
  beforeEach(() => vi.clearAllMocks());

  it("marks the mutation silent only when the caller owns the error toast", async () => {
    updateFn.mockRejectedValue(new Error("boom"));
    const { useUpdatePlaylist } = await import("./playlists");
    const { qc, wrapper } = createWrapper();
    const owned = renderHook(() => useUpdatePlaylist({ ownErrorToast: true }), { wrapper });
    const plain = renderHook(() => useUpdatePlaylist(), { wrapper });
    await act(async () => {
      await owned.result.current.mutateAsync({ id: "p-1", rules: [] }).catch(() => {});
      await plain.result.current.mutateAsync({ id: "p-2", rules: [] }).catch(() => {});
    });
    const metas = qc.getMutationCache().getAll().map((m) => m.meta?.silentError === true);
    expect(metas).toEqual([true, false]);
  });
});

// A playlist mixes videos and music tracks, told apart only by mediaType: a
// screen that cannot see it sends a track id to the video page, which can
// only answer "not found".
describe("usePlaylistDetail", () => {
  beforeEach(() => vi.clearAllMocks());
  it("keeps each item's media type", async () => {
    mockGet.mockReturnValue({ data: { id: "p-1", name: "Mixed", type: "PLAYLIST_TYPE_MANUAL" }, isLoading: false, isSuccess: true } as never);
    mockGetItems.mockReturnValue({
      data: { items: [{ mediaItemId: "m-1", mediaType: "video", title: "A film" }, { mediaItemId: "t-1", mediaType: "music" }] },
      isLoading: false,
      isSuccess: true,
    } as never);
    const { usePlaylistDetail } = await import("./playlists");
    const { wrapper } = createWrapper();
    const { result } = renderHook(() => usePlaylistDetail("p-1"), { wrapper });
    expect(result.current.data?.items.map((i) => i.media_type)).toEqual(["video", "music"]);
  });
});
