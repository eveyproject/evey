import { describe, it, expect, vi, beforeEach } from "vitest";

const eveyFetchMock = vi.fn();
vi.mock("@evey/api", () => ({ eveyFetch: (...args: unknown[]) => eveyFetchMock(...args) }));

import { useCollection, useMediaItemCollections } from "./collections";
import { renderHook, waitFor, act } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

beforeEach(() => { eveyFetchMock.mockReset(); });

describe("useMediaItemCollections", () => {
  it("fetches the sagas a title belongs to", async () => {
    const data = { collections: [{ id: "c1", name: "Alien", itemCount: 4 }] };
    eveyFetchMock.mockResolvedValue(data);

    const { result } = renderHook(() => useMediaItemCollections("m1"), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/media/m1/collections",
      method: "GET",
    });
    expect(result.current.data).toEqual(data);
  });

  it("is disabled when mediaItemId is empty", () => {
    const { result } = renderHook(() => useMediaItemCollections(""), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe("idle");
  });
});

// A collection larger than one page used to show its first 50 titles with
// nothing saying the rest existed.
describe("useCollection", () => {
  it("asks for the next page and hands back every title loaded so far", async () => {
    eveyFetchMock.mockImplementation(({ url }: { url: string }) =>
      Promise.resolve(
        url.includes("offset=0")
          ? { collection: { id: "c2", name: "Long" }, items: [{ id: "m1" }, { id: "m2" }], total: 3 }
          : { collection: { id: "c2", name: "Long" }, items: [{ id: "m3" }], total: 3 },
      ),
    );

    const { result } = renderHook(() => useCollection("c2", 2), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.items).toHaveLength(2);
    expect(result.current.hasNextPage).toBe(true);

    await act(async () => {
      await result.current.fetchNextPage();
    });

    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/collections/c2?limit=2&offset=2",
      method: "GET",
    });
    await waitFor(() => expect(result.current.data?.items).toHaveLength(3));
    expect(result.current.hasNextPage).toBe(false);
  });
});
