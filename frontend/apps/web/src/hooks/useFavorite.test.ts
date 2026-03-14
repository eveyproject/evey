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
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

describe("useIsFavorite", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("returns true when API says favorited", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({ isFavorite: true });

    const { useIsFavorite } = await import("./useFavorite");
    const { result } = renderHook(() => useIsFavorite("media-1"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toBe(true);
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/api/v1/favorites/media-1", method: "GET" }),
    );
  });

  it("is disabled when mediaId undefined", async () => {
    const fetch = await getEveyFetch();

    const { useIsFavorite } = await import("./useFavorite");
    const { result } = renderHook(() => useIsFavorite(undefined), {
      wrapper: createWrapper(),
    });

    expect(result.current.fetchStatus).toBe("idle");
    expect(fetch).not.toHaveBeenCalled();
  });
});

describe("useToggleFavorite", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls POST on mutate", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({ isFavorite: true });

    const { useToggleFavorite } = await import("./useFavorite");
    const { result } = renderHook(() => useToggleFavorite("media-2"), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate();
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/api/v1/favorites/media-2", method: "POST" }),
    );
  });

  it("rolls back on error", async () => {
    const fetch = await getEveyFetch();
    fetch.mockRejectedValueOnce(new Error("server error"));

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
