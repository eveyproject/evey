import React from "react";
import { renderHook, act, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, it, expect, vi, beforeEach } from "vitest";

const mockIsFav = vi.fn(() => ({ data: false }));
const mockAdd = vi.fn();
const mockRemove = vi.fn();

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  useFavoriteServiceIsFavorite: (_id: string, _p: unknown, opts: { query?: { enabled?: boolean; select?: (d: unknown) => unknown } }) => {
    const raw = mockIsFav();
    const enabled = opts?.query?.enabled !== false;
    const select = opts?.query?.select;
    return { ...raw, data: enabled && raw?.data !== undefined && select ? select({ isFavorite: raw.data }) : (enabled ? raw?.data : undefined) };
  },
  useFavoriteServiceListFavorites: () => ({ data: [] }),
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
