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

describe("useStarRating", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("returns 0 when no rating", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({ value: 0 });

    const { useStarRating } = await import("./useStarRating");
    const { result } = renderHook(() => useStarRating("media-1"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.rating).toBe(0));
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/api/v1/media/media-1/rating", method: "GET" }),
    );
  });

  it("returns current rating from API", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({ value: 4 });

    const { useStarRating } = await import("./useStarRating");
    const { result } = renderHook(() => useStarRating("media-2"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.rating).toBe(4));
  });

  it("is disabled when mediaId is undefined", async () => {
    const fetch = await getEveyFetch();

    const { useStarRating } = await import("./useStarRating");
    const { result } = renderHook(() => useStarRating(undefined), {
      wrapper: createWrapper(),
    });

    expect(result.current.rating).toBe(0);
    expect(fetch).not.toHaveBeenCalled();
  });

  it("setRating calls POST with the given value", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValue({ value: 3 });

    const { useStarRating } = await import("./useStarRating");
    const { result } = renderHook(() => useStarRating("media-3"), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.setRating(3);
    });

    await waitFor(() =>
      expect(fetch).toHaveBeenCalledWith(
        expect.objectContaining({ url: "/api/v1/media/media-3/rating", method: "POST", data: { value: 3 } }),
      ),
    );
  });

  it("setRating calls DELETE when toggling off (same value clicked)", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({ value: 3 });
    fetch.mockResolvedValueOnce({ value: 0 });

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    qc.setQueryData(["star-rating", "media-4"], { value: 3 });
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(QueryClientProvider, { client: qc }, children);

    const { useStarRating } = await import("./useStarRating");
    const { result } = renderHook(() => useStarRating("media-4"), { wrapper });

    await waitFor(() => expect(result.current.rating).toBe(3));

    await act(async () => {
      result.current.setRating(3); // clicking same star removes it
    });

    await waitFor(() =>
      expect(fetch).toHaveBeenCalledWith(
        expect.objectContaining({ url: "/api/v1/media/media-4/rating", method: "DELETE" }),
      ),
    );
  });

  it("rolls back optimistic update on error", async () => {
    const fetch = await getEveyFetch();
    fetch.mockRejectedValueOnce(new Error("server error"));

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    qc.setQueryData(["star-rating", "media-5"], { value: 2 });
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(QueryClientProvider, { client: qc }, children);

    const { useStarRating } = await import("./useStarRating");
    const { result } = renderHook(() => useStarRating("media-5"), { wrapper });

    await waitFor(() => expect(result.current.rating).toBe(2));

    await act(async () => {
      result.current.setRating(5);
    });

    await waitFor(() => expect(result.current.rating).toBe(2));
  });
});

describe("useRatingAggregate", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("returns aggregate data from API", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({ average: 4.2, count: 15 });

    const { useRatingAggregate } = await import("./useStarRating");
    const { result } = renderHook(() => useRatingAggregate("media-6"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual({ average: 4.2, count: 15 });
  });

  it("is disabled when mediaId is undefined", async () => {
    const fetch = await getEveyFetch();

    const { useRatingAggregate } = await import("./useStarRating");
    const { result } = renderHook(() => useRatingAggregate(undefined), {
      wrapper: createWrapper(),
    });

    expect(result.current.fetchStatus).toBe("idle");
    expect(fetch).not.toHaveBeenCalled();
  });
});
