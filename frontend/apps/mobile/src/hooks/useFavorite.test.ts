import React from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const eveyFetchMock = vi.fn();
vi.mock("@evey/api", () => ({ eveyFetch: (...args: unknown[]) => eveyFetchMock(...args) }));

import { useIsFavorite, useToggleFavorite } from "./useFavorite";

function makeWrapper(qc?: QueryClient) {
  const client = qc ?? new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client }, children);
}

beforeEach(() => { eveyFetchMock.mockReset(); });

describe("useIsFavorite", () => {
  it("returns true when API responds isFavorite=true (select transform)", async () => {
    eveyFetchMock.mockResolvedValueOnce({ isFavorite: true });

    const { result } = renderHook(() => useIsFavorite("media-1"), { wrapper: makeWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toBe(true);
    expect(eveyFetchMock).toHaveBeenCalledWith({ url: "/api/v1/favorites/media-1", method: "GET" });
  });

  it("returns false when API responds isFavorite=false", async () => {
    eveyFetchMock.mockResolvedValueOnce({ isFavorite: false });

    const { result } = renderHook(() => useIsFavorite("media-2"), { wrapper: makeWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toBe(false);
  });

  it("is disabled when mediaId is undefined", () => {
    const { result } = renderHook(() => useIsFavorite(undefined), { wrapper: makeWrapper() });

    expect(result.current.fetchStatus).toBe("idle");
    expect(eveyFetchMock).not.toHaveBeenCalled();
  });

  it("is disabled when mediaId is empty string", () => {
    const { result } = renderHook(() => useIsFavorite(""), { wrapper: makeWrapper() });

    expect(result.current.fetchStatus).toBe("idle");
    expect(eveyFetchMock).not.toHaveBeenCalled();
  });
});

describe("useToggleFavorite", () => {
  it("calls POST to favorites endpoint on mutate", async () => {
    eveyFetchMock.mockResolvedValueOnce({ isFavorite: true });

    const { result } = renderHook(() => useToggleFavorite("media-3"), { wrapper: makeWrapper() });

    await act(async () => { result.current.mutate(); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(eveyFetchMock).toHaveBeenCalledWith({ url: "/api/v1/favorites/media-3", method: "POST" });
  });

  it("optimistically toggles isFavorite before response", async () => {
    let resolve!: (v: unknown) => void;
    eveyFetchMock.mockReturnValueOnce(new Promise((r) => { resolve = r; }));

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    qc.setQueryData(["favorite", "media-4"], { isFavorite: false });

    const { result } = renderHook(() => useToggleFavorite("media-4"), { wrapper: makeWrapper(qc) });

    await act(async () => { result.current.mutate(); });

    // optimistic update flips to true before server responds
    expect(qc.getQueryData(["favorite", "media-4"])).toEqual({ isFavorite: true });

    resolve({ isFavorite: true });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });

  it("rolls back on server error", async () => {
    eveyFetchMock.mockRejectedValueOnce(new Error("server error"));

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    qc.setQueryData(["favorite", "media-5"], { isFavorite: true });

    const { result } = renderHook(() => useToggleFavorite("media-5"), { wrapper: makeWrapper(qc) });

    await act(async () => { result.current.mutate(); });
    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(qc.getQueryData(["favorite", "media-5"])).toEqual({ isFavorite: true });
  });
});
