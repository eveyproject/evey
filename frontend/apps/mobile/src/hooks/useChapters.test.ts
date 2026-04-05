import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const eveyFetchMock = vi.fn();
vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
}));

import { useChapters } from "./useChapters";

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

beforeEach(() => { eveyFetchMock.mockReset(); });

describe("useChapters", () => {
  it("fetches chapters for a media item", async () => {
    const chapters = [
      { title: "Intro", startTimeMs: 0, endTimeMs: 60000 },
      { title: "Main", startTimeMs: 60000, endTimeMs: 300000 },
    ];
    eveyFetchMock.mockResolvedValue({ chapters });

    const { result } = renderHook(() => useChapters("m1"), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/media/m1/chapters",
      method: "GET",
    });
    expect(result.current.data).toEqual(chapters);
  });

  it("returns empty array when no chapters", async () => {
    eveyFetchMock.mockResolvedValue({ chapters: null });

    const { result } = renderHook(() => useChapters("m2"), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual([]);
  });

  it("returns empty array on error", async () => {
    eveyFetchMock.mockRejectedValue(new Error("network"));

    const { result } = renderHook(() => useChapters("m3"), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual([]);
  });

  it("does not fetch when mediaId is undefined", () => {
    renderHook(() => useChapters(undefined), { wrapper: createWrapper() });
    expect(eveyFetchMock).not.toHaveBeenCalled();
  });
});
