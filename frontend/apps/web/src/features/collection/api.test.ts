import React from "react";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, it, expect, vi, beforeEach } from "vitest";

const mockList = vi.fn(() => ({ data: { collections: [] } }));
const mockGet = vi.fn(() => ({ data: undefined, isLoading: false, isSuccess: true }));
const mockGetItems = vi.fn(() => ({ data: { items: [], total: 0 }, isLoading: false, isSuccess: true }));
const deleteFn = vi.fn();
const eveyFetchFn = vi.fn();

function sel<T>(raw: { data?: T; [k: string]: unknown }, opts: { query?: { select?: (d: T) => unknown; enabled?: boolean } }) {
  const s = opts?.query?.select;
  const enabled = opts?.query?.enabled !== false;
  return { ...raw, data: enabled && raw?.data !== undefined && s ? s(raw.data) : (enabled ? raw?.data : undefined), fetchStatus: enabled ? "idle" : "idle" };
}

vi.mock("@evey/api", () => ({
  eveyFetch: eveyFetchFn,
  useCollectionServiceListCollections: (opts: { query?: { select?: (d: unknown) => unknown } }) => sel(mockList(), opts),
  useCollectionServiceGetCollection: (_id: string, opts: { query?: { select?: (d: unknown) => unknown; enabled?: boolean } }) => sel(mockGet(), opts),
  useCollectionServiceGetCollectionItems: (_id: string, _p: unknown, opts: { query?: { select?: (d: unknown) => unknown; enabled?: boolean } }) => sel(mockGetItems(), opts),
  collectionServiceDeleteCollection: (id: string) => deleteFn(id),
  getCollectionServiceListCollectionsQueryKey: () => ["collections"],
  getCollectionServiceGetCollectionQueryKey: (id: string) => ["collection", id],
  getCollectionServiceGetCollectionItemsQueryKey: (id: string) => ["collection-items", id],
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return { qc, wrapper: ({ children }: { children: React.ReactNode }) => React.createElement(QueryClientProvider, { client: qc }, children) };
}

describe("useCollections", () => {
  beforeEach(() => vi.clearAllMocks());

  it("returns list of collections", async () => {
    mockList.mockReturnValue({ data: { collections: [{ id: "c-1", name: "Marvel", createdAt: "2024-01-01", updatedAt: "2024-01-01" }] } });
    const { useCollections } = await import("./api");
    const { wrapper } = createWrapper();
    const { result } = renderHook(() => useCollections(), { wrapper });
    expect(result.current.data?.collections[0]?.id).toBe("c-1");
  });
});

describe("useCollection", () => {
  beforeEach(() => vi.clearAllMocks());

  it("fetches collection detail with items", async () => {
    mockGet.mockReturnValue({ data: { collection: { id: "c-1", name: "Marvel", createdAt: "2024-01-01", updatedAt: "2024-01-01" } }, isLoading: false, isSuccess: true });
    mockGetItems.mockReturnValue({ data: { items: [{ id: "m-1", title: "Iron Man", type: "movie" }], total: 1 }, isLoading: false, isSuccess: true });
    const { useCollection } = await import("./api");
    const { wrapper } = createWrapper();
    const { result } = renderHook(() => useCollection("c-1"), { wrapper });
    expect(result.current.data?.collection.name).toBe("Marvel");
    expect(result.current.data?.items[0]?.title).toBe("Iron Man");
  });

  it("is disabled when id is empty", async () => {
    mockGet.mockReturnValue({ data: undefined, isLoading: false, isSuccess: false });
    mockGetItems.mockReturnValue({ data: undefined, isLoading: false, isSuccess: false });
    const { useCollection } = await import("./api");
    const { wrapper } = createWrapper();
    const { result } = renderHook(() => useCollection(""), { wrapper });
    expect(result.current.data).toBeUndefined();
  });
});

describe("useCreateCollection", () => {
  beforeEach(() => vi.clearAllMocks());

  it("calls POST and invalidates collections query", async () => {
    eveyFetchFn.mockResolvedValueOnce({ id: "new-1" });
    const { useCreateCollection } = await import("./api");
    const { qc, wrapper } = createWrapper();
    const spy = vi.spyOn(qc, "invalidateQueries");
    const { result } = renderHook(() => useCreateCollection(), { wrapper });
    await act(async () => { result.current.mutate({ name: "New" }); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(eveyFetchFn).toHaveBeenCalled();
    expect(spy).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["collections"] }));
  });
});

describe("useUpdateCollection", () => {
  beforeEach(() => vi.clearAllMocks());

  it("calls PUT and invalidates collections query", async () => {
    eveyFetchFn.mockResolvedValueOnce(undefined);
    const { useUpdateCollection } = await import("./api");
    const { qc, wrapper } = createWrapper();
    const spy = vi.spyOn(qc, "invalidateQueries");
    const { result } = renderHook(() => useUpdateCollection(), { wrapper });
    await act(async () => { result.current.mutate({ id: "c-1", name: "Updated" }); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(eveyFetchFn).toHaveBeenCalled();
    expect(spy).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["collections"] }));
  });
});

describe("useDeleteCollection", () => {
  beforeEach(() => vi.clearAllMocks());

  it("calls DELETE and invalidates collections query", async () => {
    deleteFn.mockResolvedValueOnce(undefined);
    const { useDeleteCollection } = await import("./api");
    const { qc, wrapper } = createWrapper();
    const spy = vi.spyOn(qc, "invalidateQueries");
    const { result } = renderHook(() => useDeleteCollection(), { wrapper });
    await act(async () => { result.current.mutate("c-1"); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(deleteFn).toHaveBeenCalledWith("c-1");
    expect(spy).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["collections"] }));
  });
});
