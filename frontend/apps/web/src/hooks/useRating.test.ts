import React from "react";
import { renderHook, act, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, it, expect, vi, beforeEach } from "vitest";

const mockUseGet = vi.fn();
const mockPut = vi.fn();

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  useRatingServiceGetMyThumbRating: (id: string, opts: { query?: { enabled?: boolean; select?: (d: unknown) => unknown } }) => {
    const raw = mockUseGet(id);
    const select = opts?.query?.select;
    const enabled = opts?.query?.enabled !== false;
    return {
      ...raw,
      data: enabled && raw?.data !== undefined && select ? select(raw.data) : (enabled ? raw?.data : undefined),
    };
  },
  ratingServicePutThumbRating: (id: string, body: unknown) => mockPut(id, body),
  getRatingServiceGetMyThumbRatingQueryKey: (id: string) => ["rating", id],
}));

function wrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

describe("useMyRating", () => {
  beforeEach(() => vi.clearAllMocks());

  it("fetches rating for mediaId", async () => {
    mockUseGet.mockReturnValue({ data: { value: 1 }, isSuccess: true });
    const { useMyRating } = await import("./useRating");
    const { result } = renderHook(() => useMyRating("m-1"), { wrapper: wrapper() });
    expect(result.current.data).toBe(1);
  });

  it("is disabled when mediaId is undefined", async () => {
    mockUseGet.mockReturnValue({ data: undefined });
    const { useMyRating } = await import("./useRating");
    const { result } = renderHook(() => useMyRating(undefined), { wrapper: wrapper() });
    expect(result.current.data).toBeUndefined();
  });

  it("returns -1 for dislike", async () => {
    mockUseGet.mockReturnValue({ data: { value: -1 }, isSuccess: true });
    const { useMyRating } = await import("./useRating");
    const { result } = renderHook(() => useMyRating("m-2"), { wrapper: wrapper() });
    expect(result.current.data).toBe(-1);
  });
});

describe("useSetRating", () => {
  beforeEach(() => vi.clearAllMocks());

  it("calls put with rating value", async () => {
    mockPut.mockResolvedValueOnce({ value: 1 });
    mockUseGet.mockReturnValue({ data: undefined });
    const { useSetRating } = await import("./useRating");
    const { result } = renderHook(() => useSetRating("m-3"), { wrapper: wrapper() });
    await act(async () => { result.current.mutate(1); });
    await waitFor(() => expect(mockPut).toHaveBeenCalledWith("m-3", { value: 1 }));
  });
});
