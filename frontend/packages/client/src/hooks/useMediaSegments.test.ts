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

  it("returns the segments array from the response", async () => {
    const segments = [
      { type: "intro", startMs: 0, endMs: 30000, confidence: 0.9, source: "audio" },
    ];
    eveyFetchMock.mockResolvedValueOnce({ segments });

    const { result } = renderHook(() => useMediaSegments("mi-3"), {
      wrapper: makeWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual({ segments, introAutoSkip: false });
  });

  it("returns an empty result when the fetch throws (error swallowed by hook)", async () => {
    eveyFetchMock.mockRejectedValueOnce(new Error("network error"));

    const { result } = renderHook(() => useMediaSegments("mi-4"), {
      wrapper: makeWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual({ segments: [], introAutoSkip: false });
  });

  it("returns an empty result when the response has no segments field", async () => {
    eveyFetchMock.mockResolvedValueOnce({});

    const { result } = renderHook(() => useMediaSegments("mi-5"), {
      wrapper: makeWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual({ segments: [], introAutoSkip: false });
  });
  // The admin auto-skip toggle rides in the same response. The TV app was the
  // one player that never read it, so the setting silently did nothing here.
  it("reads the admin auto-skip flag out of the response", async () => {
    eveyFetchMock.mockResolvedValueOnce({ segments: [], introAutoSkip: true });

    const { result } = renderHook(() => useMediaSegments("mi-6"), { wrapper: makeWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.introAutoSkip).toBe(true);
  });

  // Fail closed: an absent flag must never read as "auto-skip enabled".
  it("treats an absent or non-true flag as off", async () => {
    eveyFetchMock.mockResolvedValueOnce({ segments: [], introAutoSkip: "yes" });

    const { result } = renderHook(() => useMediaSegments("mi-7"), { wrapper: makeWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.introAutoSkip).toBe(false);
  });
});
