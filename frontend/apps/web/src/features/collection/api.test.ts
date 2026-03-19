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
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

describe("useCollections", () => {
  beforeEach(() => vi.clearAllMocks());

  it("returns list of collections", async () => {
    const fetch = await getEveyFetch();
    const collections = [
      { id: "c-1", name: "Marvel", createdAt: "2024-01-01", updatedAt: "2024-01-01" },
    ];
    fetch.mockResolvedValueOnce({ collections });

    const { useCollections } = await import("./api");
    const { result } = renderHook(() => useCollections(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.collections).toEqual(collections);
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/api/v1/collections", method: "GET" }),
    );
  });
});

describe("useCollection", () => {
  beforeEach(() => vi.clearAllMocks());

  it("fetches collection detail with items", async () => {
    const fetch = await getEveyFetch();
    const payload = {
      collection: { id: "c-1", name: "Marvel", createdAt: "2024-01-01", updatedAt: "2024-01-01" },
      items: [{ id: "m-1", title: "Iron Man", type: "movie" }],
      total: 1,
    };
    fetch.mockResolvedValueOnce(payload);

    const { useCollection } = await import("./api");
    const { result } = renderHook(() => useCollection("c-1"), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.collection.name).toBe("Marvel");
    expect(result.current.data?.items).toHaveLength(1);
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/api/v1/collections/c-1?limit=50&offset=0", method: "GET" }),
    );
  });

  it("is disabled when id is empty", async () => {
    const fetch = await getEveyFetch();

    const { useCollection } = await import("./api");
    const { result } = renderHook(() => useCollection(""), { wrapper: createWrapper() });

    expect(result.current.fetchStatus).toBe("idle");
    expect(fetch).not.toHaveBeenCalled();
  });
});

describe("useCreateCollection", () => {
  beforeEach(() => vi.clearAllMocks());

  it("calls POST and invalidates collections query", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({ id: "new-1" });

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(QueryClientProvider, { client: qc }, children);

    const { useCreateCollection } = await import("./api");
    const { result } = renderHook(() => useCreateCollection(), { wrapper });

    await act(async () => {
      result.current.mutate({ name: "Action Movies", rules: [{ field: "genre", operator: "eq", value: "Action" }] });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({
        url: "/api/v1/collections",
        method: "POST",
        data: { name: "Action Movies", rules: [{ field: "genre", operator: "eq", value: "Action" }] },
      }),
    );
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["collections"] }),
    );
  });
});

describe("useUpdateCollection", () => {
  beforeEach(() => vi.clearAllMocks());

  it("calls PUT and invalidates collections query", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce(undefined);

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(QueryClientProvider, { client: qc }, children);

    const { useUpdateCollection } = await import("./api");
    const { result } = renderHook(() => useUpdateCollection(), { wrapper });

    await act(async () => {
      result.current.mutate({ id: "c-1", name: "Updated", rules: [] });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({
        url: "/api/v1/collections/c-1",
        method: "PUT",
        data: { name: "Updated", rules: [] },
      }),
    );
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["collections"] }),
    );
  });
});

describe("useDeleteCollection", () => {
  beforeEach(() => vi.clearAllMocks());

  it("calls DELETE and invalidates collections query", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce(undefined);

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(QueryClientProvider, { client: qc }, children);

    const { useDeleteCollection } = await import("./api");
    const { result } = renderHook(() => useDeleteCollection(), { wrapper });

    await act(async () => {
      result.current.mutate("c-1");
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/api/v1/collections/c-1", method: "DELETE" }),
    );
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["collections"] }),
    );
  });
});
