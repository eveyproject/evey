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
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

describe("useAdminMediaSegments", () => {
  beforeEach(() => { vi.clearAllMocks(); });

  it("queries all admin segment types", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({ segments: [] });

    const { useAdminMediaSegments } = await import("./useAdminSegments");
    const { result } = renderHook(() => useAdminMediaSegments("media-1"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({
        url: expect.stringContaining("intro,recap,end_credits,studio_logo"),
        method: "GET",
      }),
    );
  });
});

describe("useSetSegment", () => {
  beforeEach(() => { vi.clearAllMocks(); });

  it("calls PUT /api/v1/admin/media/{id}/segments with correct body", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({});

    const { useSetSegment } = await import("./useAdminSegments");
    const { result } = renderHook(() => useSetSegment("media-1"), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      await result.current.mutateAsync({ type: "intro", startMs: 1000, endMs: 5000 });
    });

    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({
        url: "/api/v1/admin/media/media-1/segments",
        method: "PUT",
        data: { type: "intro", startMs: 1000, endMs: 5000 },
      }),
    );
  });

  it("invalidates segments query on success", async () => {
    const fetch = await getEveyFetch();
    // First call: initial query (useAdminMediaSegments would call this, but we skip that here)
    fetch.mockResolvedValueOnce({});

    const { useSetSegment } = await import("./useAdminSegments");
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(QueryClientProvider, { client: qc }, children);

    const { result } = renderHook(() => useSetSegment("media-abc"), { wrapper });

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

describe("useDeleteSegment", () => {
  beforeEach(() => { vi.clearAllMocks(); });

  it("calls DELETE with type query param", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({});

    const { useDeleteSegment } = await import("./useAdminSegments");
    const { result } = renderHook(() => useDeleteSegment("media-2"), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      await result.current.mutateAsync("intro");
    });

    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({
        url: "/api/v1/admin/media/media-2/segments?type=intro",
        method: "DELETE",
      }),
    );
  });
});

describe("usePropagateSegment", () => {
  beforeEach(() => { vi.clearAllMocks(); });

  it("calls POST propagate with type query param and returns response", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({ propagatedTo: ["ep1", "ep2"], count: 2 });

    const { usePropagateSegment } = await import("./useAdminSegments");
    const { result } = renderHook(() => usePropagateSegment("media-3"), {
      wrapper: createWrapper(),
    });

    let res: { propagatedTo: string[]; count: number } | undefined;
    await act(async () => {
      res = await result.current.mutateAsync("intro") as { propagatedTo: string[]; count: number };
    });

    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({
        url: "/api/v1/admin/media/media-3/segments/propagate?type=intro",
        method: "POST",
      }),
    );
    expect(res?.count).toBe(2);
  });
});

describe("useTriggerAutoDetect", () => {
  beforeEach(() => { vi.clearAllMocks(); });

  it("calls POST /detect-segments", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({});

    const { useTriggerAutoDetect } = await import("./useAdminSegments");
    const { result } = renderHook(() => useTriggerAutoDetect("media-4"), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      await result.current.mutateAsync();
    });

    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({
        url: "/api/v1/admin/media/media-4/detect-segments",
        method: "POST",
      }),
    );
  });
});
