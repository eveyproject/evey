// The collections of every client (web, phone, both televisions) since
// 2026-09-24; this suite came from the web's own layer.
import React from "react";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, it, expect, vi, beforeEach } from "vitest";

const mockList = vi.fn(() => ({ data: { collections: [] } }));
const mockGet = vi.fn(() => ({ data: undefined, isLoading: false, isSuccess: true }));
const mockGetItems = vi.fn(() => ({ data: { items: [], total: 0 }, isLoading: false, isSuccess: true }));
const deleteFn = vi.fn();
const createFn = vi.fn();
const updateFn = vi.fn();
const addItemFn = vi.fn();
const getItemsFn = vi.fn();

function sel<T>(raw: { data?: T; [k: string]: unknown }, opts: { query?: { select?: (d: T) => unknown; enabled?: boolean } }) {
  const s = opts?.query?.select;
  const enabled = opts?.query?.enabled !== false;
  return { ...raw, data: enabled && raw?.data !== undefined && s ? s(raw.data) : (enabled ? raw?.data : undefined), fetchStatus: enabled ? "idle" : "idle" };
}

vi.mock("@evey/api", () => ({
  // Signatures mirror the generated hooks: both requests carry parameters, so
  // options come third for a path-parameter hook and second for a plain list.
  useCollectionServiceListCollections: (_p: unknown, opts: { query?: { select?: (d: unknown) => unknown } }) => sel(mockList(), opts),
  useCollectionServiceGetCollection: (_id: string, _p: unknown, opts: { query?: { select?: (d: unknown) => unknown; enabled?: boolean } }) => sel(mockGet(), opts),
  useCollectionServiceGetCollectionItems: (_id: string, _p: unknown, opts: { query?: { select?: (d: unknown) => unknown; enabled?: boolean } }) => sel(mockGetItems(), opts),
  collectionServiceCreateCollection: (body: unknown) => createFn(body),
  collectionServiceUpdateCollection: (id: string, body: unknown) => updateFn(id, body),
  collectionServiceDeleteCollection: (id: string) => deleteFn(id),
  collectionServiceAddCollectionItem: (id: string, body: unknown) => addItemFn(id, body),
  getCollectionServiceListCollectionsQueryKey: () => ["collections"],
  getCollectionServiceGetCollectionQueryKey: (id: string) => ["collection", id],
  getCollectionServiceGetCollectionItemsQueryKey: (id: string) => ["collection-items", id],
  useCollectionServiceListCollectionsForMediaItem: (_id: string, opts: { query?: { select?: (d: unknown) => unknown } }) => sel(mockList(), opts),
  getCollectionServiceListCollectionsForMediaItemQueryKey: (id: string) => ["media-collections", id],
  collectionServiceGetCollectionItems: (id: string, params: unknown) => getItemsFn(id, params),
  collectionServiceRemoveCollectionItem: vi.fn(),
  collectionServiceReorderCollectionItems: vi.fn(),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return { qc, wrapper: ({ children }: { children: React.ReactNode }) => React.createElement(QueryClientProvider, { client: qc }, children) };
}

describe("useCollections", () => {
  beforeEach(() => vi.clearAllMocks());

  it("returns list of collections", async () => {
    mockList.mockReturnValue({ data: { collections: [{ id: "c-1", name: "Marvel", createdAt: "2024-01-01", updatedAt: "2024-01-01" }] } });
    const { useCollections } = await import("./collections");
    const { wrapper } = createWrapper();
    const { result } = renderHook(() => useCollections(), { wrapper });
    expect(result.current.data?.collections[0]?.id).toBe("c-1");
  });
});

describe("useCollectionPage", () => {
  beforeEach(() => vi.clearAllMocks());

  it("fetches collection detail with items", async () => {
    mockGet.mockReturnValue({ data: { collection: { id: "c-1", name: "Marvel", createdAt: "2024-01-01", updatedAt: "2024-01-01" } }, isLoading: false, isSuccess: true });
    mockGetItems.mockReturnValue({ data: { items: [{ id: "m-1", title: "Iron Man", type: "movie" }], total: 1 }, isLoading: false, isSuccess: true });
    const { useCollectionPage: useCollection } = await import("./collections");
    const { wrapper } = createWrapper();
    const { result } = renderHook(() => useCollection("c-1"), { wrapper });
    expect(result.current.data?.collection.name).toBe("Marvel");
    expect(result.current.data?.items[0]?.title).toBe("Iron Man");
  });

  it("is disabled when id is empty", async () => {
    mockGet.mockReturnValue({ data: undefined, isLoading: false, isSuccess: false });
    mockGetItems.mockReturnValue({ data: undefined, isLoading: false, isSuccess: false });
    const { useCollectionPage: useCollection } = await import("./collections");
    const { wrapper } = createWrapper();
    const { result } = renderHook(() => useCollection(""), { wrapper });
    expect(result.current.data).toBeUndefined();
  });
});

describe("useCreateCollection", () => {
  beforeEach(() => vi.clearAllMocks());

  it("calls CreateCollection RPC and invalidates collections query", async () => {
    createFn.mockResolvedValueOnce({ collection: { id: "new-1" } });
    const { useCreateCollection } = await import("./collections");
    const { qc, wrapper } = createWrapper();
    const spy = vi.spyOn(qc, "invalidateQueries");
    const { result } = renderHook(() => useCreateCollection(), { wrapper });
    await act(async () => { result.current.mutate({ name: "New" }); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(createFn).toHaveBeenCalledWith(expect.objectContaining({ name: "New", smart: false }));
    expect(spy).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["collections"] }));
  });

  it("infers smart=true when rules are provided", async () => {
    createFn.mockResolvedValueOnce({ collection: { id: "new-2" } });
    const { useCreateCollection } = await import("./collections");
    const { wrapper } = createWrapper();
    const { result } = renderHook(() => useCreateCollection(), { wrapper });
    await act(async () => {
      result.current.mutate({ name: "Smart", rules: [{ field: "genre", operator: "eq", value: "drama" }] });
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(createFn).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "Smart",
        smart: true,
        rules: [{ field: "genre", operator: "eq", value: "drama" }],
      })
    );
  });
});

describe("useUpdateCollection", () => {
  beforeEach(() => vi.clearAllMocks());

  it("calls UpdateCollection RPC and invalidates collections query", async () => {
    updateFn.mockResolvedValueOnce({ collection: { id: "c-1" } });
    const { useUpdateCollection } = await import("./collections");
    const { qc, wrapper } = createWrapper();
    const spy = vi.spyOn(qc, "invalidateQueries");
    const { result } = renderHook(() => useUpdateCollection(), { wrapper });
    await act(async () => { result.current.mutate({ id: "c-1", name: "Updated" }); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(updateFn).toHaveBeenCalledWith(
      "c-1",
      expect.objectContaining({ name: "Updated", smart: false })
    );
    expect(spy).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["collections"] }));
    expect(spy).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["collection", "c-1"] }));
    // A smart collection's rules decide its items, so editing must refresh the
    // items query too - otherwise the collection shows stale matches.
    expect(spy).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["collection-items", "c-1"] }));
  });
});

describe("useDeleteCollection", () => {
  beforeEach(() => vi.clearAllMocks());

  it("calls DELETE and invalidates collections query", async () => {
    deleteFn.mockResolvedValueOnce(undefined);
    const { useDeleteCollection } = await import("./collections");
    const { qc, wrapper } = createWrapper();
    const spy = vi.spyOn(qc, "invalidateQueries");
    const { result } = renderHook(() => useDeleteCollection(), { wrapper });
    await act(async () => { result.current.mutate("c-1"); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(deleteFn).toHaveBeenCalledWith("c-1");
    expect(spy).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["collections"] }));
  });
});

describe("addCollectionItem", () => {
  beforeEach(() => vi.clearAllMocks());

  it("calls AddCollectionItem RPC with mediaItemId and sortOrder", async () => {
    addItemFn.mockResolvedValueOnce({});
    const { addCollectionItem } = await import("./collections");
    await addCollectionItem("c-1", "m-1");
    expect(addItemFn).toHaveBeenCalledWith("c-1", { mediaItemId: "m-1", sortOrder: 0 });
  });
});

// What the phone's and televisions' former copy relied on.
describe("useCollection (a growing list, for scrollers)", () => {
  beforeEach(() => vi.clearAllMocks());

  it("asks for the next page and hands back every title loaded so far", async () => {
    mockGet.mockReturnValue({ data: { collection: { id: "c-1", name: "Bond Saga" } }, isLoading: false, isSuccess: true } as never);
    getItemsFn
      .mockResolvedValueOnce({ items: [{ id: "m1", title: "One" }, { id: "m2", title: "Two" }], total: 3 })
      .mockResolvedValueOnce({ items: [{ id: "m3", title: "Three" }], total: 3 });
    const { useCollection } = await import("./collections");
    const { wrapper } = createWrapper();
    const { result } = renderHook(() => useCollection("c-1", 2), { wrapper });
    await waitFor(() => expect(result.current.data?.items).toHaveLength(2));
    expect(result.current.hasNextPage).toBe(true);
    await act(async () => { await result.current.fetchNextPage(); });
    await waitFor(() => expect(result.current.data?.items.map((i) => i.id)).toEqual(["m1", "m2", "m3"]));
    expect(getItemsFn).toHaveBeenLastCalledWith("c-1", { limit: 2, offset: 2 });
    expect(result.current.hasNextPage).toBe(false);
    expect(result.current.data?.collection.name).toBe("Bond Saga");
  });
});

describe("useCreateCollection's answer", () => {
  beforeEach(() => vi.clearAllMocks());

  // Measured on the dev server: the new collection comes back wrapped.
  it("returns the id of the collection it created", async () => {
    createFn.mockResolvedValue({ collection: { id: "c-9", name: "New" } });
    const { useCreateCollection } = await import("./collections");
    const { wrapper } = createWrapper();
    const { result } = renderHook(() => useCreateCollection(), { wrapper });
    let created: { id: string } | undefined;
    await act(async () => { created = await result.current.mutateAsync({ name: "New" }); });
    expect(created).toEqual({ id: "c-9" });
  });
});
