import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const api = vi.hoisted(() => ({
  listTracks: vi.fn(),
  libOpts: null as any,
  albumOpts: null as any,
  artistOpts: null as any,
}));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  musicServiceListTracks: (...a: unknown[]) => api.listTracks(...a),
  useLibraryServiceListLibraries: (o: any) => {
    api.libOpts = o.query;
    return { data: [] };
  },
  useMusicServiceGetAlbums: (_p: unknown, o: any) => {
    api.albumOpts = o.query;
    return { data: [] };
  },
  useMusicServiceGetArtists: (_p: unknown, o: any) => {
    api.artistOpts = o.query;
    return { data: [] };
  },
}));

import { useMusicLibraries, useMusicTracks, useMusicAlbums, useMusicArtists } from "./musicQueries";

let qc: QueryClient;
const wrapper = ({ children }: { children: React.ReactNode }) => React.createElement(QueryClientProvider, { client: qc }, children);

beforeEach(() => {
  qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  api.listTracks.mockReset();
});

// Tracks page by keyset: the first request is offset 0, the next carries the
// server's cursor, and the pages are flattened into one list. Only the plain
// listing pages - an album or a search comes back whole, with no cursor, so
// there is no next page to ask for. Nothing is asked without a library.
describe("useMusicTracks", () => {
  it("walks the cursor and flattens the pages", async () => {
    api.listTracks
      .mockResolvedValueOnce({ tracks: [{ id: "t1", title: "One" }, { id: "t2" }], nextCursor: "c1" })
      .mockResolvedValueOnce({ tracks: [{ id: "t3", title: "Three", year: 1999 }] });
    const { result } = renderHook(() => useMusicTracks("lib-1", ""), { wrapper });
    await waitFor(() => expect(result.current.data).toHaveLength(2));
    expect(api.listTracks.mock.calls[0][0]).toEqual({ libraryId: "lib-1", limit: 200, offset: 0 });
    expect(result.current.hasNextPage).toBe(true);
    expect(result.current.data[1]).toMatchObject({ ID: "t2", Title: "", Year: 0, CoverArtURL: "" });

    await act(async () => {
      await result.current.fetchNextPage();
    });
    await waitFor(() => expect(result.current.data).toHaveLength(3));
    expect(api.listTracks.mock.calls[1][0]).toEqual({ libraryId: "lib-1", limit: 200, cursor: "c1" });
    expect(result.current.hasNextPage).toBe(false);
    expect(result.current.data[2]).toMatchObject({ ID: "t3", Title: "Three", Year: 1999 });
  });

  it("an album ignores the search and comes back as one page; a search is sent as-is", async () => {
    api.listTracks.mockResolvedValue({ tracks: [{ id: "a" }] });
    const album = renderHook(() => useMusicTracks("lib-1", "ignored", "alb-1"), { wrapper });
    await waitFor(() => expect(album.result.current.data).toHaveLength(1));
    expect(api.listTracks.mock.calls[0][0]).toEqual({ libraryId: "lib-1", limit: 200, albumId: "alb-1", offset: 0 });
    expect(album.result.current.hasNextPage).toBe(false);

    const search = renderHook(() => useMusicTracks("lib-1", "heat"), { wrapper });
    await waitFor(() => expect(search.result.current.data).toHaveLength(1));
    expect(api.listTracks.mock.calls[1][0]).toEqual({ libraryId: "lib-1", limit: 200, search: "heat", offset: 0 });
  });

  it("asks nothing without a library", async () => {
    const { result } = renderHook(() => useMusicTracks(undefined, ""), { wrapper });
    await act(async () => {});
    expect(api.listTracks).not.toHaveBeenCalled();
    expect(result.current.data).toEqual([]);
  });
});

// The selectors keep only music libraries and normalise every optional
// server field to a value the tabs can render without guards.
describe("music selectors", () => {
  it("keeps music libraries only", () => {
    renderHook(() => useMusicLibraries(), { wrapper });
    expect(api.libOpts.select({ libraries: [{ id: "m", name: "Music", type: "LIBRARY_TYPE_MUSIC" }, { id: "v", type: "LIBRARY_TYPE_MOVIES" }] }))
      .toEqual([{ id: "m", name: "Music", type: "LIBRARY_TYPE_MUSIC" }]);
    expect(api.libOpts.select({})).toEqual([]);
  });

  it("normalises albums and artists, and only queries with a library", () => {
    renderHook(() => useMusicAlbums(undefined), { wrapper });
    expect(api.albumOpts.enabled).toBe(false);
    expect(api.albumOpts.select({ albums: [{ id: "a1", title: "T", trackCount: 3 }] })).toEqual([
      { ID: "a1", Title: "T", Artist: "", Year: 0, CoverArtURL: "", TrackCount: 3, Duration: 0 },
    ]);
    renderHook(() => useMusicArtists("lib-1"), { wrapper });
    expect(api.artistOpts.enabled).toBe(true);
    expect(api.artistOpts.select({})).toEqual([]);
    expect(api.artistOpts.select({ artists: [{ name: "Band", albumCount: 2 }] })).toEqual([
      { ID: "", Name: "Band", TrackCount: 0, AlbumCount: 2, CoverArtURL: "" },
    ]);
  });
});
