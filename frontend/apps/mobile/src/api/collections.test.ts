import { describe, it, expect, vi, beforeEach } from "vitest";

const eveyFetchMock = vi.fn();
vi.mock("@evey/api", () => ({ eveyFetch: (...args: unknown[]) => eveyFetchMock(...args) }));

import { useCollections, useCollection, useMediaItemCollections, useReorderCollectionItems } from "./collections";
import { renderHook, waitFor, act } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

beforeEach(() => { eveyFetchMock.mockReset(); });

describe("useCollections", () => {
  it("fetches /api/v1/collections with GET", async () => {
    const data = { collections: [{ id: "c1", name: "Action" }] };
    eveyFetchMock.mockResolvedValue(data);

    const { result } = renderHook(() => useCollections(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/collections",
      method: "GET",
    });
    expect(result.current.data).toEqual(data);
  });
});

describe("useCollection", () => {
  it("fetches collection by id with default limit/offset", async () => {
    const data = { collection: { id: "c1", name: "Action" }, items: [], total: 0 };
    eveyFetchMock.mockResolvedValue(data);

    const { result } = renderHook(() => useCollection("c1"), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/collections/c1?limit=50&offset=0",
      method: "GET",
    });
  });

  it("passes custom limit and offset", async () => {
    eveyFetchMock.mockResolvedValue({ collection: {}, items: [], total: 0 });

    const { result } = renderHook(() => useCollection("c2", 10, 20), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/collections/c2?limit=10&offset=20",
      method: "GET",
    });
  });

  it("is disabled when id is empty", () => {
    const { result } = renderHook(() => useCollection(""), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe("idle");
  });
});

describe("useReorderCollectionItems", () => {
  it("PUTs the full new order, then flips sortBy to manual", async () => {
    eveyFetchMock.mockResolvedValue(undefined);

    const { result } = renderHook(() => useReorderCollectionItems("c1"), { wrapper: createWrapper() });
    await act(async () => {
      await result.current.mutateAsync({ mediaItemIds: ["mi-1", "mi-2", "mi-3"], name: "Bond Saga" });
    });

    expect(eveyFetchMock).toHaveBeenNthCalledWith(1, {
      url: "/api/v1/collections/c1/items/order",
      method: "PUT",
      data: { mediaItemIds: ["mi-1", "mi-2", "mi-3"] },
    });
    expect(eveyFetchMock).toHaveBeenNthCalledWith(2, {
      url: "/api/v1/collections/c1",
      method: "PUT",
      data: { name: "Bond Saga", sortBy: "manual" },
    });
  });
});

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
