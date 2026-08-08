import React from "react";
import { renderHook, act, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, it, expect, vi, beforeEach } from "vitest";

const mockIsFav = vi.fn(() => ({ data: false }));
const mockAdd = vi.fn();
const mockRemove = vi.fn();

const mockListFavorites = vi.fn();

// Spreads the real module: this hook file also imports collectAllPages and
// EXPORT_PAGE_SIZE from @evey/api, and a mock that lists only the endpoints
// would leave them undefined - the failure mode that has bitten every screen
// test in this repo that mocked this package wholesale.
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: vi.fn(),
  useFavoriteServiceIsFavorite: (_id: string, _p: unknown, opts: { query?: { enabled?: boolean; select?: (d: unknown) => unknown } }) => {
    const raw = mockIsFav();
    const enabled = opts?.query?.enabled !== false;
    const select = opts?.query?.select;
    return { ...raw, data: enabled && raw?.data !== undefined && select ? select({ isFavorite: raw.data }) : (enabled ? raw?.data : undefined) };
  },
  favoriteServiceListFavorites: (params: unknown, signal?: AbortSignal) => mockListFavorites(params, signal),
  favoriteServiceAddFavorite: (id: string, body: unknown) => mockAdd(id, body),
  favoriteServiceRemoveFavorite: (id: string) => mockRemove(id),
  getFavoriteServiceIsFavoriteQueryKey: (id: string) => ["favorite", id],
  getFavoriteServiceListFavoritesQueryKey: () => ["favorites"],
}));

function wrap() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

describe("useIsFavorite", () => {
  beforeEach(() => vi.clearAllMocks());

  it("returns true when API says favorited", async () => {
    mockIsFav.mockReturnValue({ data: true });
    const { useIsFavorite } = await import("./useFavorite");
    const { result } = renderHook(() => useIsFavorite("media-1"), { wrapper: wrap() });
    expect(result.current.data).toBe(true);
  });

  it("is disabled when mediaId undefined", async () => {
    mockIsFav.mockReturnValue({ data: undefined });
    const { useIsFavorite } = await import("./useFavorite");
    const { result } = renderHook(() => useIsFavorite(undefined), { wrapper: wrap() });
    expect(result.current.data).toBeUndefined();
  });
});

describe("useToggleFavorite", () => {
  beforeEach(() => vi.clearAllMocks());

  it("calls add on mutate", async () => {
    mockIsFav.mockReturnValue({ data: false });
    mockAdd.mockResolvedValue({ isFavorite: true });
    const { useToggleFavorite } = await import("./useFavorite");
    const { result } = renderHook(() => useToggleFavorite("media-2"), { wrapper: wrap() });
    await act(async () => { result.current.mutate(false); });
    await waitFor(() => expect(mockAdd).toHaveBeenCalledWith("media-2", {}));
  });
});

describe("useFavoritesList", () => {
  beforeEach(() => vi.clearAllMocks());

  // The favourites page sorts, filters and searches client-side, so it needs
  // every row. The hook used to send no pagination and get the handler's
  // default of 50 back, with no total in the response to reveal the cut: a
  // library of 530 favourites rendered 50, and searching the other 480
  // answered "no results".
  it("walks every page instead of stopping at the first one", async () => {
    const page = (n: number, from: number) =>
      Array.from({ length: n }, (_, i) => ({ mediaItemId: `m-${from + i}`, createdAt: "2026-01-01" }));
    mockListFavorites
      .mockResolvedValueOnce({ items: page(500, 0) })
      .mockResolvedValueOnce({ items: page(30, 500) });

    const { useFavoritesList } = await import("./useFavorite");
    const { result } = renderHook(() => useFavoritesList(), { wrapper: wrap() });

    await waitFor(() => expect(result.current.data).toHaveLength(530));
    expect(mockListFavorites).toHaveBeenCalledTimes(2);
    // Second call must move the window forward, not re-ask for page one.
    expect(mockListFavorites.mock.calls[0][0]).toMatchObject({ limit: 500, offset: 0 });
    expect(mockListFavorites.mock.calls[1][0]).toMatchObject({ limit: 500, offset: 500 });
    expect(result.current.data?.[529]).toMatchObject({ mediaItemId: "m-529" });
  });

  it("stops after a single short page", async () => {
    mockListFavorites.mockResolvedValueOnce({ items: [{ mediaItemId: "m-1", createdAt: "2026-01-01" }] });
    const { useFavoritesList } = await import("./useFavorite");
    const { result } = renderHook(() => useFavoritesList(), { wrapper: wrap() });

    await waitFor(() => expect(result.current.data).toHaveLength(1));
    expect(mockListFavorites).toHaveBeenCalledTimes(1);
  });
});
