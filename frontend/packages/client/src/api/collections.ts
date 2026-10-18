import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  useCollectionServiceListCollections,
  useCollectionServiceGetCollection,
  useCollectionServiceGetCollectionItems,
  useCollectionServiceListCollectionsForMediaItem,
  collectionServiceCreateCollection,
  collectionServiceUpdateCollection,
  collectionServiceDeleteCollection,
  collectionServiceAddCollectionItem,
  collectionServiceRemoveCollectionItem,
  collectionServiceReorderCollectionItems,
  collectionServiceGetCollectionItems,
  getCollectionServiceListCollectionsQueryKey,
  getCollectionServiceGetCollectionQueryKey,
  getCollectionServiceGetCollectionItemsQueryKey,
  getCollectionServiceListCollectionsForMediaItemQueryKey,
} from "@evey/api";
import type { V1CollectionProto } from "@evey/api";

// The collections of every client. The web kept its own layer beside this one
// until 2026-09-24 (generated API, no toast) while the phone and the
// televisions used hand-written calls with the messages inside. One layer now,
// and it says nothing: the screen that calls it words its own message.

export interface CollectionRule {
  field: string;
  operator: string;
  value: string;
}

export interface Collection {
  id: string;
  tmdbId?: number;
  name: string;
  overview?: string;
  posterUrl?: string;
  backdropUrl?: string;
  rules?: CollectionRule[];
  sortBy?: string;
  createdAt: string;
  updatedAt: string;
  /** Members THIS viewer can see - already access- and parental-filtered. */
  itemCount?: number;
  /** "saga" (TMDB-sourced), "smart" (rule-driven) or "manual". */
  kind?: string;
}

export interface CollectionItem {
  id: string;
  title: string;
  type: string;
  year?: number;
  overview?: string;
  communityRating?: number;
  genres?: string[];
}

function mapCollection(c: V1CollectionProto): Collection {
  return {
    id: c.id ?? "",
    tmdbId: c.tmdbId,
    name: c.name ?? "",
    overview: c.overview,
    posterUrl: c.posterUrl,
    backdropUrl: c.backdropUrl,
    rules: c.rules?.map((r) => ({ field: r.field ?? "", operator: r.operator ?? "", value: r.value ?? "" })),
    sortBy: c.sortBy,
    itemCount: c.itemCount,
    kind: c.kind,
    createdAt: c.createdAt ?? "",
    updatedAt: c.updatedAt ?? "",
  };
}

function mapItem(i: { id?: string; title?: string; type?: string; year?: number; overview?: string; communityRating?: number; genres?: string[] }): CollectionItem {
  return {
    id: i.id ?? "",
    title: i.title ?? "",
    type: i.type ?? "",
    year: i.year,
    overview: i.overview,
    communityRating: i.communityRating,
    genres: i.genres,
  };
}

const selectList = (d: { collections?: V1CollectionProto[] }) => ({ collections: (d.collections ?? []).map(mapCollection) });

export function useCollections() {
  return useCollectionServiceListCollections(undefined, { query: { select: selectList, staleTime: 5 * 60_000 } });
}

/**
 * Every collection, including the auto-created sagas the browse listing hides
 * for holding a single title. A curation surface must show them: hiding the
 * saga you own one film of is right when browsing and wrong when the user is
 * trying to add the second one to it.
 */
export function useAllCollections() {
  return useCollectionServiceListCollections({ minItems: 1 }, { query: { select: selectList, staleTime: 5 * 60_000 } });
}

/**
 * The sagas a title belongs to, already access- and parental-filtered by the
 * server. An empty list is the common case - a title belonging to nothing.
 */
export function useMediaItemCollections(mediaItemId: string) {
  return useCollectionServiceListCollectionsForMediaItem(mediaItemId, {
    query: {
      queryKey: getCollectionServiceListCollectionsForMediaItemQueryKey(mediaItemId),
      select: selectList,
      enabled: !!mediaItemId,
      staleTime: 5 * 60_000,
    },
  });
}

function useCollectionHead(id: string) {
  return useCollectionServiceGetCollection(id, undefined, {
    query: {
      enabled: !!id,
      staleTime: 5 * 60_000,
      select: (d) => (d.collection ? mapCollection(d.collection) : undefined),
    },
  });
}

/**
 * One page of a collection's members, for a numbered pager (the web).
 */
export function useCollectionPage(id: string, limit = 50, offset = 0) {
  const head = useCollectionHead(id);
  const items = useCollectionServiceGetCollectionItems(id, { limit, offset }, {
    query: {
      enabled: !!id,
      staleTime: 5 * 60_000,
      select: (d) => ({ items: (d.items ?? []).map(mapItem), total: d.total ?? 0 }),
    },
  });
  return {
    data: head.data ? { collection: head.data, items: items.data?.items ?? [], total: items.data?.total ?? 0 } : undefined,
    isLoading: head.isLoading || items.isLoading,
    isError: head.isError || items.isError,
    error: head.error ?? items.error,
    // Both halves, so a retry after a failure fetches whichever one failed.
    refetch: () => { void head.refetch(); void items.refetch(); },
  };
}

const COLLECTION_PAGE_SIZE = 50;

/**
 * A collection and its members as one growing list, for a scroller (the
 * phone, the televisions). It has to page: the endpoint answers 50 members at
 * a time, and asking for one page showed 50 titles of a larger collection with
 * nothing saying the rest existed.
 */
export function useCollection(id: string, pageSize = COLLECTION_PAGE_SIZE) {
  const head = useCollectionHead(id);
  const items = useInfiniteQuery({
    // Under the items key, so a mutation invalidating the collection's items
    // refreshes this list too.
    queryKey: [...getCollectionServiceGetCollectionItemsQueryKey(id), "all", pageSize],
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) => collectionServiceGetCollectionItems(id, { limit: pageSize, offset: pageParam }, signal),
    getNextPageParam: (_last, pages) => {
      const loaded = pages.reduce((n, p) => n + (p.items?.length ?? 0), 0);
      return loaded < (pages[0]?.total ?? 0) ? loaded : undefined;
    },
    enabled: !!id,
    staleTime: 5 * 60_000,
  });
  const pages = items.data?.pages ?? [];
  return {
    data: head.data && pages.length
      ? { collection: head.data, items: pages.flatMap((p) => (p.items ?? []).map(mapItem)), total: pages[0]?.total ?? 0 }
      : undefined,
    isLoading: head.isLoading || items.isLoading,
    isError: head.isError || items.isError,
    error: head.error ?? items.error,
    refetch: () => { void head.refetch(); void items.refetch(); },
    fetchNextPage: items.fetchNextPage,
    hasNextPage: items.hasNextPage,
    isFetchingNextPage: items.isFetchingNextPage,
  };
}

function useInvalidate() {
  const qc = useQueryClient();
  return {
    list: () => qc.invalidateQueries({ queryKey: getCollectionServiceListCollectionsQueryKey() }),
    head: (id: string) => qc.invalidateQueries({ queryKey: getCollectionServiceGetCollectionQueryKey(id) }),
    items: (id: string) => qc.invalidateQueries({ queryKey: getCollectionServiceGetCollectionItemsQueryKey(id) }),
  };
}

interface CreateCollectionInput {
  name: string;
  overview?: string;
  rules?: CollectionRule[];
  sortBy?: string;
  smart?: boolean;
}

interface UpdateCollectionInput {
  id: string;
  name: string;
  /** Absent keeps the existing synopsis server-side; only send it when the
   * user actually touched the description field. */
  overview?: string;
  rules?: CollectionRule[];
  sortBy?: string;
  smart?: boolean;
}

export function useCreateCollection() {
  const invalidate = useInvalidate();
  return useMutation({
    // The server field is overview, not description. The answer wraps the new
    // collection ({ collection: {...} }): the phone read a flat id and never
    // opened the collection it had just created.
    mutationFn: async (input: CreateCollectionInput): Promise<{ id: string }> => {
      const created = await collectionServiceCreateCollection({
        name: input.name,
        overview: input.overview,
        rules: input.rules?.map((r) => ({ field: r.field, operator: r.operator, value: r.value })),
        sortBy: input.sortBy,
        smart: input.smart ?? (input.rules != null && input.rules.length > 0),
      });
      return { id: created.collection?.id ?? "" };
    },
    onSuccess: () => invalidate.list(),
  });
}

export function useUpdateCollection() {
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({ id, name, overview, rules, sortBy, smart }: UpdateCollectionInput) =>
      collectionServiceUpdateCollection(id, {
        name,
        overview,
        rules: rules?.map((r) => ({ field: r.field, operator: r.operator, value: r.value })),
        sortBy,
        smart: smart ?? (rules != null && rules.length > 0),
      }),
    onSuccess: (_d, vars) => {
      void invalidate.list();
      void invalidate.head(vars.id);
      // A smart collection's rules decide which items match, so editing them
      // changes the items list - refresh it too, not just the metadata.
      void invalidate.items(vars.id);
    },
  });
}

export function useDeleteCollection() {
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (id: string) => collectionServiceDeleteCollection(id),
    onSuccess: () => invalidate.list(),
  });
}

export function addCollectionItem(collectionId: string, mediaItemId: string) {
  return collectionServiceAddCollectionItem(collectionId, { mediaItemId, sortOrder: 0 });
}

/**
 * Removes one title from a collection (admin-only server-side; the title
 * itself stays). Adding shipped without this for a long time, so a wrong
 * member was permanent and curation was write-only.
 */
export function useRemoveCollectionItem(collectionId: string) {
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (mediaItemId: string) => collectionServiceRemoveCollectionItem(collectionId, mediaItemId),
    onSuccess: () => {
      void invalidate.items(collectionId);
      void invalidate.head(collectionId);
      void invalidate.list();
    },
  });
}

/**
 * Writes the hand-set order of a collection's members.
 *
 * The order only takes effect once the collection reads by it, so this also
 * switches sortBy to "manual" - a viewer who moves a title into place means
 * that place, not "store it and keep showing release order".
 */
export function useReorderCollectionItems(collectionId: string) {
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: async ({ mediaItemIds, name }: { mediaItemIds: string[]; name: string }) => {
      await collectionServiceReorderCollectionItems(collectionId, { mediaItemIds });
      await collectionServiceUpdateCollection(collectionId, { name, sortBy: "manual" });
    },
    onSuccess: () => {
      void invalidate.items(collectionId);
      void invalidate.head(collectionId);
    },
  });
}
