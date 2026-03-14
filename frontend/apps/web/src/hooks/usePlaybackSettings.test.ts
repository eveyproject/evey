import { describe, it, expect, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { usePlaybackSettings } from "./usePlaybackSettings";

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
}));

async function getEveyFetch() {
  const { eveyFetch } = await import("@evey/api");
  return vi.mocked(eveyFetch);
}

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

describe("usePlaybackSettings", () => {
  it("returns settings from API", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({ skipForwardSeconds: 30, skipBackwardSeconds: 15 });

    const { result } = renderHook(() => usePlaybackSettings(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.skipForwardSeconds).toBe(30);
    expect(result.current.data?.skipBackwardSeconds).toBe(15);
  });

  it("returns defaults on API error", async () => {
    const fetch = await getEveyFetch();
    fetch.mockRejectedValueOnce(new Error("network error"));

    const { result } = renderHook(() => usePlaybackSettings(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.skipForwardSeconds).toBe(10);
    expect(result.current.data?.skipBackwardSeconds).toBe(10);
  });

  it("applies defaults for missing fields", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({});

    const { result } = renderHook(() => usePlaybackSettings(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.skipForwardSeconds).toBe(10);
    expect(result.current.data?.skipBackwardSeconds).toBe(10);
  });
});
