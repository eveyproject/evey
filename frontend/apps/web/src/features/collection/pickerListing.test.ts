import { describe, it, expect, vi } from "vitest";

// A curation surface must see every collection, including the auto sagas the
// browse listing hides for holding a single title: hiding "Alien - Saga" is
// right while browsing and wrong when the user is adding the second film TO
// it. The picker therefore asks with min_items=1, the browse list does not.
const listSpy = vi.fn(() => ({ data: undefined, isLoading: true }));

vi.mock("@evey/api", () => ({
  useCollectionServiceListCollections: (...args: unknown[]) => listSpy(...(args as [])),
  useCollectionServiceGetCollection: vi.fn(),
  useCollectionServiceGetCollectionItems: vi.fn(),
  collectionServiceCreateCollection: vi.fn(),
  collectionServiceUpdateCollection: vi.fn(),
  collectionServiceDeleteCollection: vi.fn(),
  collectionServiceAddCollectionItem: vi.fn(),
  collectionServiceRemoveCollectionItem: vi.fn(),
  collectionServiceReorderCollectionItems: vi.fn(),
  getCollectionServiceListCollectionsQueryKey: () => ["collections"],
  getCollectionServiceGetCollectionQueryKey: () => ["collection"],
  getCollectionServiceGetCollectionItemsQueryKey: () => ["collection-items"],
}));
vi.mock("@tanstack/react-query", () => ({
  useMutation: vi.fn(),
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
}));

describe("which listing each surface asks for", () => {
  it("the browse listing takes the server default (one-title sagas hidden)", async () => {
    const { useCollections } = await import("./api");
    listSpy.mockClear();
    useCollections();
    expect(listSpy.mock.calls[0][0]).toBeUndefined();
  });

  it("the curation listing asks for everything", async () => {
    const { useAllCollections } = await import("./api");
    listSpy.mockClear();
    useAllCollections();
    expect(listSpy.mock.calls[0][0]).toEqual({ minItems: 1 });
  });
});
