import React from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const eveyFetchMock = vi.fn();
vi.mock("@evey/api", () => ({ eveyFetch: (...args: unknown[]) => eveyFetchMock(...args) }));

import {
  useAdminMediaSegments,
  useSetSegment,
  useDeleteSegment,
  usePropagateSegment,
  useTriggerAutoDetect,
} from "./useAdminSegments";

function makeWrapper(qc?: QueryClient) {
  const client =
    qc ??
    new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client }, children);
}

beforeEach(() => {
  eveyFetchMock.mockReset();
});

// ----------------------------------------------------------------------------
// useAdminMediaSegments
// ----------------------------------------------------------------------------

describe("useAdminMediaSegments", () => {
  it("fetches segments and passes all admin types in the URL", async () => {
    eveyFetchMock.mockResolvedValueOnce({ segments: [] });

    const { result } = renderHook(() => useAdminMediaSegments("media-1"), {
      wrapper: makeWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(eveyFetchMock).toHaveBeenCalledWith(
      expect.objectContaining({
        url: expect.stringContaining("intro,recap,end_credits,studio_logo"),
        method: "GET",
      }),
    );
  });

  it("is disabled when mediaId is undefined", () => {
    const { result } = renderHook(() => useAdminMediaSegments(undefined), {
      wrapper: makeWrapper(),
    });

    expect(result.current.fetchStatus).toBe("idle");
    expect(eveyFetchMock).not.toHaveBeenCalled();
  });
});

// ----------------------------------------------------------------------------
// useSetSegment
// ----------------------------------------------------------------------------

describe("useSetSegment", () => {
  it("calls PUT /api/v1/admin/media/{id}/segments with the correct payload", async () => {
    eveyFetchMock.mockResolvedValueOnce({});

    const { result } = renderHook(() => useSetSegment("media-1"), {
      wrapper: makeWrapper(),
    });

    await act(async () => {
      await result.current.mutateAsync({ type: "intro", startMs: 1000, endMs: 5000 });
    });

    expect(eveyFetchMock).toHaveBeenCalledWith(
      expect.objectContaining({
        url: "/api/v1/admin/media/media-1/segments",
        method: "PUT",
        data: { type: "intro", startMs: 1000, endMs: 5000 },
      }),
    );
  });

  it("invalidates the segments query on success", async () => {
    eveyFetchMock.mockResolvedValueOnce({});

    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");

    const { result } = renderHook(() => useSetSegment("media-abc"), {
      wrapper: makeWrapper(qc),
    });

    await act(async () => {
      await result.current.mutateAsync({ type: "recap", startMs: 0, endMs: 3000 });
    });

    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        queryKey: expect.arrayContaining(["/api/v1/media/segments", "media-abc"]),
      }),
    );
  });
});

// ----------------------------------------------------------------------------
// useDeleteSegment
// ----------------------------------------------------------------------------

describe("useDeleteSegment", () => {
  it("calls DELETE with type as query param", async () => {
    eveyFetchMock.mockResolvedValueOnce({});

    const { result } = renderHook(() => useDeleteSegment("media-2"), {
      wrapper: makeWrapper(),
    });

    await act(async () => {
      await result.current.mutateAsync("intro");
    });

    expect(eveyFetchMock).toHaveBeenCalledWith(
      expect.objectContaining({
        url: "/api/v1/admin/media/media-2/segments?type=intro",
        method: "DELETE",
      }),
    );
  });

  it("invalidates the segments query on success", async () => {
    eveyFetchMock.mockResolvedValueOnce({});

    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");

    const { result } = renderHook(() => useDeleteSegment("media-2"), {
      wrapper: makeWrapper(qc),
    });

    await act(async () => {
      await result.current.mutateAsync("end_credits");
    });

    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        queryKey: expect.arrayContaining(["/api/v1/media/segments", "media-2"]),
      }),
    );
  });
});

// ----------------------------------------------------------------------------
// usePropagateSegment
// ----------------------------------------------------------------------------

describe("usePropagateSegment", () => {
  it("calls POST propagate with type as query param and returns the response", async () => {
    eveyFetchMock.mockResolvedValueOnce({ propagatedTo: ["ep1", "ep2"], count: 2 });

    const { result } = renderHook(() => usePropagateSegment("media-3"), {
      wrapper: makeWrapper(),
    });

    let res: { propagatedTo: string[]; count: number } | undefined;
    await act(async () => {
      res = (await result.current.mutateAsync("intro")) as {
        propagatedTo: string[];
        count: number;
      };
    });

    expect(eveyFetchMock).toHaveBeenCalledWith(
      expect.objectContaining({
        url: "/api/v1/admin/media/media-3/segments/propagate?type=intro",
        method: "POST",
      }),
    );
    expect(res?.count).toBe(2);
    // usePropagateSegment has no onSuccess invalidation in the hook implementation.
  });
});

// ----------------------------------------------------------------------------
// useTriggerAutoDetect
// ----------------------------------------------------------------------------

describe("useTriggerAutoDetect", () => {
  it("calls POST /detect-segments for the given media", async () => {
    eveyFetchMock.mockResolvedValueOnce({});

    const { result } = renderHook(() => useTriggerAutoDetect("media-4"), {
      wrapper: makeWrapper(),
    });

    await act(async () => {
      await result.current.mutateAsync();
    });

    expect(eveyFetchMock).toHaveBeenCalledWith(
      expect.objectContaining({
        url: "/api/v1/admin/media/media-4/detect-segments",
        method: "POST",
      }),
    );
    // useTriggerAutoDetect has no onSuccess invalidation in the hook implementation.
  });
});
