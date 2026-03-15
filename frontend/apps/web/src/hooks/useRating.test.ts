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

describe("useMyRating", () => {
  beforeEach(() => vi.clearAllMocks());

  it("fetches rating for mediaId", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({ value: 1 });

    const { useMyRating } = await import("./useRating");
    const { result } = renderHook(() => useMyRating("m-1"), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toBe(1);
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/api/v1/media/m-1/my-rating", method: "GET" }),
    );
  });

  it("is disabled when mediaId is undefined", async () => {
    const fetch = await getEveyFetch();

    const { useMyRating } = await import("./useRating");
    const { result } = renderHook(() => useMyRating(undefined), { wrapper: createWrapper() });

    expect(result.current.fetchStatus).toBe("idle");
    expect(fetch).not.toHaveBeenCalled();
  });

  it("returns -1 for dislike", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({ value: -1 });

    const { useMyRating } = await import("./useRating");
    const { result } = renderHook(() => useMyRating("m-2"), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toBe(-1);
  });
});

describe("useSetRating", () => {
  beforeEach(() => vi.clearAllMocks());

  it("calls PUT with rating value", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({ value: 1 });

    const { useSetRating } = await import("./useRating");
    const { result } = renderHook(() => useSetRating("m-3"), { wrapper: createWrapper() });

    await act(async () => {
      result.current.mutate(1);
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/api/v1/media/m-3/rate", method: "PUT", data: { value: 1 } }),
    );
  });

  it("applies optimistic update and rolls back on error", async () => {
    const fetch = await getEveyFetch();
    fetch.mockRejectedValueOnce(new Error("fail"));

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    qc.setQueryData(["rating", "m-4"], { value: 0 });
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(QueryClientProvider, { client: qc }, children);

    const { useSetRating } = await import("./useRating");
    const { result } = renderHook(() => useSetRating("m-4"), { wrapper });

    await act(async () => {
      result.current.mutate(1);
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(qc.getQueryData(["rating", "m-4"])).toEqual({ value: 0 });
  });
});
