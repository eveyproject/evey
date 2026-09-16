import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const eveyFetchMock = vi.fn();
vi.mock("@evey/api", () => ({ eveyFetch: (...args: unknown[]) => eveyFetchMock(...args) }));

import { useMostPopular } from "./usePopular";

function wrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => React.createElement(QueryClientProvider, { client: qc }, children);
}

beforeEach(() => {
  eveyFetchMock.mockReset();
});

// The "most popular" row is one endpoint for the three clients: the hook
// reads the items and turns a missing array into an empty row rather than a
// crash on `.map`.
describe("useMostPopular", () => {
  it("unwraps the items", async () => {
    eveyFetchMock.mockResolvedValue({ items: [{ mediaItemId: "m1", title: "One", playCount: 3 }] });
    const { result } = renderHook(() => useMostPopular(5), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([{ mediaItemId: "m1", title: "One", playCount: 3 }]);
    expect(eveyFetchMock).toHaveBeenCalledWith({ url: "/api/v1/popular", method: "GET" });
  });

  it("reads a missing array as an empty row", async () => {
    eveyFetchMock.mockResolvedValue({});
    const { result } = renderHook(() => useMostPopular(), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });
});
