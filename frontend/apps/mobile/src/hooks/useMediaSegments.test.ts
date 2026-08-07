import React from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const eveyFetchMock = vi.fn();
vi.mock("@evey/api", () => ({ eveyFetch: (...args: unknown[]) => eveyFetchMock(...args) }));

import { useMediaSegments } from "./useMediaSegments";

function makeWrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client }, children);
}

beforeEach(() => {
  eveyFetchMock.mockReset();
});

describe("useMediaSegments", () => {
  it("uses default types when none are provided", async () => {
    eveyFetchMock.mockResolvedValueOnce({ segments: [] });

    const { result } = renderHook(() => useMediaSegments("mi-1"), {
      wrapper: makeWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(eveyFetchMock).toHaveBeenCalledWith(
      expect.objectContaining({
        url: expect.stringContaining("types=intro,outro,recap,end_credits,studio_logo"),
        method: "GET",
      }),
    );
  });

  it("uses the caller-supplied types in the URL", async () => {
    eveyFetchMock.mockResolvedValueOnce({ segments: [] });

    const { result } = renderHook(() => useMediaSegments("mi-2", ["intro", "outro"]), {
      wrapper: makeWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(eveyFetchMock).toHaveBeenCalledWith(
      expect.objectContaining({
        url: expect.stringContaining("types=intro,outro"),
        method: "GET",
      }),
    );
    // The stricter-types request must NOT contain the defaults-only types.
    const calledUrl: string = eveyFetchMock.mock.calls[0][0].url;
    expect(calledUrl).not.toContain("end_credits");
  });

  it("is disabled when mediaId is undefined", () => {
    const { result } = renderHook(() => useMediaSegments(undefined), {
      wrapper: makeWrapper(),
    });

    expect(result.current.fetchStatus).toBe("idle");
    expect(eveyFetchMock).not.toHaveBeenCalled();
  });

  it("returns the segments and the introAutoSkip flag from the response", async () => {
    const segments = [
      { type: "intro", startMs: 0, endMs: 30000, confidence: 0.9, source: "audio" },
    ];
    eveyFetchMock.mockResolvedValueOnce({ segments, introAutoSkip: true });

    const { result } = renderHook(() => useMediaSegments("mi-3"), {
      wrapper: makeWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual({ segments, introAutoSkip: true });
  });

  it("returns empty segments and autoSkip=false when the fetch throws (error swallowed by hook)", async () => {
    eveyFetchMock.mockRejectedValueOnce(new Error("network error"));

    const { result } = renderHook(() => useMediaSegments("mi-4"), {
      wrapper: makeWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual({ segments: [], introAutoSkip: false });
  });

  it("defaults both fields when the response is empty (older server)", async () => {
    eveyFetchMock.mockResolvedValueOnce({});

    const { result } = renderHook(() => useMediaSegments("mi-5"), {
      wrapper: makeWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual({ segments: [], introAutoSkip: false });
  });
});
