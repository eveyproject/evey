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
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

describe("useMediaSegments", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("uses default types when types is omitted", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({ segments: [] });

    const { useMediaSegments } = await import("./useMediaSegments");
    const { result } = renderHook(() => useMediaSegments("media-1"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({
        url: expect.stringContaining("intro,outro,recap,end_credits,studio_logo"),
        method: "GET",
      }),
    );
  });

  it("joins custom types array with commas in the URL", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({ segments: [] });

    const { useMediaSegments } = await import("./useMediaSegments");
    const { result } = renderHook(
      () => useMediaSegments("media-2", ["intro", "outro"]),
      { wrapper: createWrapper() },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({
        url: "/api/v1/media/media-2/segments?types=intro,outro",
        method: "GET",
      }),
    );
  });

  it("is disabled when mediaId is undefined", async () => {
    const fetch = await getEveyFetch();

    const { useMediaSegments } = await import("./useMediaSegments");
    const { result } = renderHook(() => useMediaSegments(undefined), {
      wrapper: createWrapper(),
    });

    expect(result.current.fetchStatus).toBe("idle");
    expect(fetch).not.toHaveBeenCalled();
  });

  it("returns empty segments and autoSkip=false on eveyFetch throw (swallowed error)", async () => {
    const fetch = await getEveyFetch();
    fetch.mockRejectedValueOnce(new Error("network error"));

    const { useMediaSegments } = await import("./useMediaSegments");
    const { result } = renderHook(() => useMediaSegments("media-3"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual({ segments: [], introAutoSkip: false });
  });

  it("returns segments and the introAutoSkip flag on success", async () => {
    const segments = [
      { type: "intro", startMs: 0, endMs: 90000, confidence: 0.95, source: "audio" },
      { type: "outro", startMs: 5400000, endMs: 5490000, confidence: 0.9, source: "chapter" },
    ];
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({ segments, introAutoSkip: true });

    const { useMediaSegments } = await import("./useMediaSegments");
    const { result } = renderHook(() => useMediaSegments("media-4"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual({ segments, introAutoSkip: true });
  });

  it("defaults introAutoSkip to false when the server omits it (older server)", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({ segments: [] });

    const { useMediaSegments } = await import("./useMediaSegments");
    const { result } = renderHook(() => useMediaSegments("media-5"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.introAutoSkip).toBe(false);
  });
});
