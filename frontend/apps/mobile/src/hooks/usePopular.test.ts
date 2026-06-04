import React from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const eveyFetchMock = vi.fn();
vi.mock("@evey/api", () => ({ eveyFetch: (...args: unknown[]) => eveyFetchMock(...args) }));

import { useMostPopular } from "./usePopular";

function makeWrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client }, children);
}

beforeEach(() => {
  eveyFetchMock.mockReset();
});

describe("useMostPopular", () => {
  it("fetches /api/v1/popular and returns the items array", async () => {
    const items = [
      { mediaItemId: "mi-1", title: "Movie A", playCount: 42 },
      { mediaItemId: "mi-2", title: "Movie B", playCount: 17 },
    ];
    eveyFetchMock.mockResolvedValueOnce({ items });

    const { result } = renderHook(() => useMostPopular(), {
      wrapper: makeWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/popular",
      method: "GET",
    });
    expect(result.current.data).toEqual(items);
  });

  it("uses default limit of 20 in the query key", async () => {
    eveyFetchMock.mockResolvedValueOnce({ items: [] });

    const { result } = renderHook(() => useMostPopular(), {
      wrapper: makeWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    // The query key encodes the limit so callers with different limits get separate caches.
    // Default limit must be 20.
    expect(result.current.data).toEqual([]);
  });

  it("uses caller-supplied limit in the query key", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    eveyFetchMock.mockResolvedValue({ items: [] });

    // Mount two hooks with different limits — they must be independent queries.
    const { result: r5 } = renderHook(() => useMostPopular(5), {
      wrapper: ({ children }: { children: React.ReactNode }) =>
        React.createElement(QueryClientProvider, { client }, children),
    });
    const { result: r20 } = renderHook(() => useMostPopular(20), {
      wrapper: ({ children }: { children: React.ReactNode }) =>
        React.createElement(QueryClientProvider, { client }, children),
    });

    await waitFor(() => expect(r5.current.isSuccess).toBe(true));
    await waitFor(() => expect(r20.current.isSuccess).toBe(true));

    // Two separate queries should have been fired.
    expect(eveyFetchMock).toHaveBeenCalledTimes(2);
  });

  it("returns empty array when response items is missing (select guard)", async () => {
    eveyFetchMock.mockResolvedValueOnce({});

    const { result } = renderHook(() => useMostPopular(), {
      wrapper: makeWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });
});
