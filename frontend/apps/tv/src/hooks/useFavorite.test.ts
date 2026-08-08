import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const eveyFetchMock = vi.fn();
// Spreads the real module: the hook also imports collectAllPages and
// EXPORT_PAGE_SIZE from @evey/api, which a mock listing only eveyFetch would
// leave undefined (enforced by packages/api/src/mockShapeGuard.test.ts).
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
}));

import { useFavoritesList } from "./useFavorite";

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

beforeEach(() => { eveyFetchMock.mockReset(); });

describe("useFavoritesList", () => {
  // The favourites screen sorts and filters on the box, so it needs every row.
  // The hook used to ask for a single page of 200, and ListFavoritesResponse
  // carries no total - so a longer list was cut with nothing to reveal it.
  it("walks every page", async () => {
    const page = (n: number, from: number) =>
      Array.from({ length: n }, (_, i) => ({ mediaItemId: `m-${from + i}`, createdAt: "2026-01-01" }));
    eveyFetchMock
      .mockResolvedValueOnce({ items: page(500, 0) })
      .mockResolvedValueOnce({ items: page(3, 500) });

    const { result } = renderHook(() => useFavoritesList(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.data).toHaveLength(503));
    expect(eveyFetchMock).toHaveBeenCalledTimes(2);
    expect(eveyFetchMock.mock.calls[0][0].url).toContain("limit=500&offset=0");
    expect(eveyFetchMock.mock.calls[1][0].url).toContain("limit=500&offset=500");
  });

  it("stops after a short first page", async () => {
    eveyFetchMock.mockResolvedValueOnce({ items: [{ mediaItemId: "m-1", createdAt: "2026-01-01" }] });

    const { result } = renderHook(() => useFavoritesList(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.data).toHaveLength(1));
    expect(eveyFetchMock).toHaveBeenCalledTimes(1);
  });
});
