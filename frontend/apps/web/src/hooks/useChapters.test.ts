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

describe("useChapters", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("returns chapters from API", async () => {
    const chapters = [
      { title: "Intro", startTimeMs: 0, endTimeMs: 90000 },
      { title: "Act 1", startTimeMs: 90000, endTimeMs: 600000 },
    ];
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({ chapters });

    const { useChapters } = await import("./useChapters");
    const { result } = renderHook(() => useChapters("media-1"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(chapters);
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/api/v1/media/media-1/chapters", method: "GET" }),
    );
  });

  it("returns empty array on error", async () => {
    const fetch = await getEveyFetch();
    fetch.mockRejectedValueOnce(new Error("not found"));

    const { useChapters } = await import("./useChapters");
    const { result } = renderHook(() => useChapters("media-2"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });

  it("returns empty array when chapters field absent", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({});

    const { useChapters } = await import("./useChapters");
    const { result } = renderHook(() => useChapters("media-3"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });

  it("is disabled when mediaId undefined", async () => {
    const fetch = await getEveyFetch();

    const { useChapters } = await import("./useChapters");
    const { result } = renderHook(() => useChapters(undefined), {
      wrapper: createWrapper(),
    });

    expect(result.current.fetchStatus).toBe("idle");
    expect(fetch).not.toHaveBeenCalled();
  });
});
