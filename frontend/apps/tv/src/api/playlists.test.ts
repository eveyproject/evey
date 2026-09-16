import { describe, it, expect, vi, beforeEach } from "vitest";

const eveyFetchMock = vi.fn();
vi.mock("@evey/api", () => ({ eveyFetch: (...args: unknown[]) => eveyFetchMock(...args) }));

import { usePlaylistList, usePlaylistDetail, useAddToPlaylist } from "./playlists";
import { renderHook, waitFor, act } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const wrapper = ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
  return { wrapper, qc };
}

beforeEach(() => {
  eveyFetchMock.mockReset();
});

describe("usePlaylistList", () => {
  it("unwraps the list and reads a missing array as empty", async () => {
    eveyFetchMock.mockResolvedValue({});
    const { result } = renderHook(() => usePlaylistList(), { wrapper: createWrapper().wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
    expect(eveyFetchMock).toHaveBeenCalledWith({ url: "/api/v1/playlists", method: "GET" });
  });
});

// The detail endpoint answers FLAT (no wrapper) and the members come from
// their own endpoint: the hook joins the two and maps camelCase to the
// snake_case the TV screens read. Claiming a wrapper once threw on open.
describe("usePlaylistDetail", () => {
  it("joins the flat playlist with its items and maps the field names", async () => {
    eveyFetchMock.mockImplementation(({ url }: { url: string }) =>
      Promise.resolve(
        url.endsWith("/items")
          ? { items: [{ mediaItemId: "m1", sortOrder: 2, addedAt: "2026-09-16", title: "One", type: "movie", year: 2001 }, {}] }
          : {
              id: "p1",
              name: "Soirée",
              description: "d",
              type: "smart",
              rules: [{ field: "genre", operator: "eq", value: "Drame" }],
              sortBy: "title",
              sortOrder: "asc",
              createdAt: "c",
              updatedAt: "u",
            },
      ),
    );
    const { result } = renderHook(() => usePlaylistDetail("p1"), { wrapper: createWrapper().wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(eveyFetchMock).toHaveBeenCalledWith({ url: "/api/v1/playlists/p1", method: "GET" });
    expect(eveyFetchMock).toHaveBeenCalledWith({ url: "/api/v1/playlists/p1/items", method: "GET" });
    expect(result.current.data).toEqual({
      id: "p1",
      name: "Soirée",
      description: "d",
      type: "smart",
      rules: [{ field: "genre", operator: "eq", value: "Drame" }],
      sort_by: "title",
      sort_order: "asc",
      created_at: "c",
      updated_at: "u",
      items: [
        { media_item_id: "m1", sort_order: 2, added_at: "2026-09-16", title: "One", type: "movie", year: 2001 },
        { media_item_id: "", sort_order: undefined, added_at: undefined, title: undefined, type: undefined, year: undefined },
      ],
    });
  });

  it("does not fetch without an id", () => {
    const { result } = renderHook(() => usePlaylistDetail(""), { wrapper: createWrapper().wrapper });
    expect(result.current.fetchStatus).toBe("idle");
    expect(eveyFetchMock).not.toHaveBeenCalled();
  });
});

describe("useAddToPlaylist", () => {
  it("posts the member and invalidates that playlist's detail", async () => {
    eveyFetchMock.mockResolvedValue(undefined);
    const { wrapper, qc } = createWrapper();
    const invalidate = vi.spyOn(qc, "invalidateQueries");
    const { result } = renderHook(() => useAddToPlaylist(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync({ playlistId: "p1", mediaItemId: "m9" });
    });
    expect(eveyFetchMock).toHaveBeenCalledWith({ url: "/api/v1/playlists/p1/items", method: "POST", data: { mediaItemId: "m9" } });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["playlists", "p1"] });
  });
});
