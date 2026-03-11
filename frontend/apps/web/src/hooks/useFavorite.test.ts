import React from "react";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, it, expect, vi, beforeEach } from "vitest";

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign(
    (selector: (s: { accessToken: string }) => unknown) =>
      selector({ accessToken: "test-token" }),
    { getState: () => ({ accessToken: "test-token" }) },
  ),
}));

function createWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

describe("useIsFavorite", () => {
  beforeEach(() => {
    mockFetch.mockReset();
  });

  it("returns true when API says favorited", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ isFavorite: true }),
    });

    const { useIsFavorite } = await import("./useFavorite");
    const { result } = renderHook(() => useIsFavorite("media-1"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toBe(true);
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/v1/favorites/media-1",
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: "Bearer test-token" }),
      }),
    );
  });

  it("is disabled when mediaId undefined", async () => {
    const { useIsFavorite } = await import("./useFavorite");
    const { result } = renderHook(() => useIsFavorite(undefined), {
      wrapper: createWrapper(),
    });

    expect(result.current.fetchStatus).toBe("idle");
    expect(mockFetch).not.toHaveBeenCalled();
  });
});

describe("useToggleFavorite", () => {
  beforeEach(() => {
    mockFetch.mockReset();
  });

  it("calls POST on mutate", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ isFavorite: true }),
    });

    const { useToggleFavorite } = await import("./useFavorite");
    const { result } = renderHook(() => useToggleFavorite("media-2"), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate();
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/v1/favorites/media-2",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("rolls back on error", async () => {
    mockFetch.mockResolvedValueOnce({ ok: false, status: 500 });

    const { useToggleFavorite } = await import("./useFavorite");
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    qc.setQueryData(["favorite", "media-3"], { isFavorite: true });

    const wrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(QueryClientProvider, { client: qc }, children);

    const { result } = renderHook(() => useToggleFavorite("media-3"), { wrapper });

    await act(async () => {
      result.current.mutate();
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(qc.getQueryData(["favorite", "media-3"])).toEqual({ isFavorite: true });
  });
});
