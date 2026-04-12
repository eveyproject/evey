import React from "react";
import { renderHook, act, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, it, expect, vi, beforeEach } from "vitest";

const mockUseStar = vi.fn();
const mockUseAgg = vi.fn();
const mockPost = vi.fn();
const mockDelete = vi.fn();

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  useRatingServiceGetStarRating: (id: string, opts: { query?: { enabled?: boolean; select?: (d: unknown) => unknown } }) => {
    const raw = mockUseStar(id);
    const select = opts?.query?.select;
    const enabled = opts?.query?.enabled !== false;
    return {
      ...raw,
      data: enabled && raw?.data !== undefined && select ? select(raw.data) : (enabled ? raw?.data : undefined),
    };
  },
  useRatingServiceGetRatingAggregate: (id: string, opts: { query?: { enabled?: boolean } }) => {
    const enabled = opts?.query?.enabled !== false;
    const raw = mockUseAgg(id);
    return { ...raw, data: enabled ? raw?.data : undefined, fetchStatus: enabled ? "idle" : "idle" };
  },
  ratingServicePostStarRating: (id: string, body: unknown) => mockPost(id, body),
  ratingServiceDeleteStarRating: (id: string) => mockDelete(id),
  getRatingServiceGetStarRatingQueryKey: (id: string) => ["star-rating", id],
  getRatingServiceGetRatingAggregateQueryKey: (id: string) => ["star-rating-agg", id],
}));

function wrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

describe("useStarRating", () => {
  beforeEach(() => vi.clearAllMocks());

  it("returns 0 when no rating", async () => {
    mockUseStar.mockReturnValue({ data: { value: 0 } });
    const { useStarRating } = await import("./useStarRating");
    const { result } = renderHook(() => useStarRating("media-1"), { wrapper: wrapper() });
    expect(result.current.rating).toBe(0);
  });

  it("returns current rating from API", async () => {
    mockUseStar.mockReturnValue({ data: { value: 4 } });
    const { useStarRating } = await import("./useStarRating");
    const { result } = renderHook(() => useStarRating("media-2"), { wrapper: wrapper() });
    expect(result.current.rating).toBe(4);
  });

  it("is disabled when mediaId is undefined", async () => {
    mockUseStar.mockReturnValue({ data: undefined });
    const { useStarRating } = await import("./useStarRating");
    const { result } = renderHook(() => useStarRating(undefined), { wrapper: wrapper() });
    expect(result.current.rating).toBe(0);
  });

  it("setRating calls POST with the given value", async () => {
    mockUseStar.mockReturnValue({ data: { value: 0 } });
    mockPost.mockResolvedValue({ value: 3 });
    const { useStarRating } = await import("./useStarRating");
    const { result } = renderHook(() => useStarRating("media-3"), { wrapper: wrapper() });
    await act(async () => { result.current.setRating(3); });
    await waitFor(() => expect(mockPost).toHaveBeenCalledWith("media-3", { value: 3 }));
  });

  it("setRating calls DELETE when toggling off", async () => {
    mockUseStar.mockReturnValue({ data: { value: 3 } });
    mockDelete.mockResolvedValue(undefined);
    const { useStarRating } = await import("./useStarRating");
    const { result } = renderHook(() => useStarRating("media-4"), { wrapper: wrapper() });
    await act(async () => { result.current.setRating(3); });
    await waitFor(() => expect(mockDelete).toHaveBeenCalledWith("media-4"));
  });
});

describe("useRatingAggregate", () => {
  beforeEach(() => vi.clearAllMocks());

  it("returns aggregate data from API", async () => {
    mockUseAgg.mockReturnValue({ data: { average: 4.2, count: 15 }, isSuccess: true });
    const { useRatingAggregate } = await import("./useStarRating");
    const { result } = renderHook(() => useRatingAggregate("media-6"), { wrapper: wrapper() });
    expect(result.current.data).toEqual({ average: 4.2, count: 15 });
  });
});
