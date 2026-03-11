import React from "react";
import { renderHook, waitFor } from "@testing-library/react";
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

describe("useChapters", () => {
  beforeEach(() => {
    mockFetch.mockReset();
  });

  it("returns chapters from API", async () => {
    const chapters = [
      { title: "Intro", startTimeMs: 0, endTimeMs: 90000 },
      { title: "Act 1", startTimeMs: 90000, endTimeMs: 600000 },
    ];
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ chapters }),
    });

    const { useChapters } = await import("./useChapters");
    const { result } = renderHook(() => useChapters("media-1"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(chapters);
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/v1/media/media-1/chapters",
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: "Bearer test-token" }),
      }),
    );
  });

  it("returns empty array on error response", async () => {
    mockFetch.mockResolvedValueOnce({ ok: false, status: 404 });

    const { useChapters } = await import("./useChapters");
    const { result } = renderHook(() => useChapters("media-2"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });

  it("returns empty array when chapters field absent", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({}),
    });

    const { useChapters } = await import("./useChapters");
    const { result } = renderHook(() => useChapters("media-3"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });

  it("is disabled when mediaId undefined", async () => {
    const { useChapters } = await import("./useChapters");
    const { result } = renderHook(() => useChapters(undefined), {
      wrapper: createWrapper(),
    });

    expect(result.current.fetchStatus).toBe("idle");
    expect(mockFetch).not.toHaveBeenCalled();
  });
});
