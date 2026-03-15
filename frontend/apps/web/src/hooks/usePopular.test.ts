import React from "react";
import { renderHook, waitFor } from "@testing-library/react";
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

describe("useMostPopular", () => {
  beforeEach(() => vi.clearAllMocks());

  it("returns items from API", async () => {
    const fetch = await getEveyFetch();
    const items = [
      { mediaItemId: "m-1", title: "Movie A", type: "movie", playCount: 42 },
      { mediaItemId: "m-2", title: "Movie B", type: "movie", playCount: 10 },
    ];
    fetch.mockResolvedValueOnce({ items });

    const { useMostPopular } = await import("./usePopular");
    const { result } = renderHook(() => useMostPopular(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(items);
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/api/v1/popular", method: "GET" }),
    );
  });

  it("returns empty array when items is missing", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({});

    const { useMostPopular } = await import("./usePopular");
    const { result } = renderHook(() => useMostPopular(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });

  it("passes limit in query key", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({ items: [] });

    const { useMostPopular } = await import("./usePopular");
    const { result } = renderHook(() => useMostPopular(5), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetch).toHaveBeenCalledTimes(1);
  });
});
