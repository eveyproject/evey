import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  useCollectionServiceListCollections,
  useCollectionServiceGetCollection,
  useCollectionServiceGetCollectionItems,
  collectionServiceCreateCollection,
  collectionServiceUpdateCollection,
  collectionServiceDeleteCollection,
  collectionServiceAddCollectionItem,
  collectionServiceRemoveCollectionItem,
  collectionServiceReorderCollectionItems,
  getCollectionServiceListCollectionsQueryKey,
  getCollectionServiceGetCollectionQueryKey,
  getCollectionServiceGetCollectionItemsQueryKey,
} from "@evey/api";
import type { V1CollectionProto } from "@evey/api";

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

export function useCollections() {
  return useCollectionServiceListCollections(undefined, {
    query: {
      select: (d) => ({ collections: (d.collections ?? []).map(mapCollection) }),
      staleTime: 5 * 60_000,
    },
  });
}

/**
 * Every collection, including the auto-created sagas the browse listing hides
 * for holding a single title. A curation surface must show them: hiding the
 * "Alien - Saga" you own one film of is right when browsing and wrong when
 * the user is trying to add the second one to it.
 */
export function useAllCollections() {
  return useCollectionServiceListCollections({ minItems: 1 }, {
    query: {
      select: (d) => ({ collections: (d.collections ?? []).map(mapCollection) }),
      staleTime: 5 * 60_000,
    },
  });
}

export function useCollection(id: string, limit = 50, offset = 0) {
  // The request carries limit/offset now, so the generated hook takes the
  // params slot before the options one - passing options first silently landed
  // them in params.
  const head = useCollectionServiceGetCollection(id, undefined, {
    query: {
      enabled: !!id,
      staleTime: 5 * 60_000,
      select: (d) => (d.collection ? mapCollection(d.collection) : undefined),
    },
  });
  const items = useCollectionServiceGetCollectionItems(id, { limit, offset }, {
    query: {
      enabled: !!id,
      staleTime: 5 * 60_000,
      select: (d) => ({
        items: (d.items ?? []).map<CollectionItem>((i) => ({
          id: i.id ?? "",
          title: i.title ?? "",
          type: i.type ?? "",
          year: i.year,
          overview: i.overview,
          communityRating: i.communityRating,
          genres: i.genres,
        })),
        total: d.total ?? 0,
      }),
    },
  });
  return {
    data: head.data ? { collection: head.data, items: items.data?.items ?? [], total: items.data?.total ?? 0 } : undefined,
    isLoading: head.isLoading || items.isLoading,
    isError: head.isError || items.isError,
    error: head.error ?? items.error,
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
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateCollectionInput) =>
      collectionServiceCreateCollection({
        name: input.name,
        overview: input.overview,
        rules: input.rules?.map((r) => ({ field: r.field, operator: r.operator, value: r.value })),
        sortBy: input.sortBy,
        smart: input.smart ?? (input.rules != null && input.rules.length > 0),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: getCollectionServiceListCollectionsQueryKey() }),
  });
}

export function useUpdateCollection() {
  const qc = useQueryClient();
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
      qc.invalidateQueries({ queryKey: getCollectionServiceListCollectionsQueryKey() });
      qc.invalidateQueries({ queryKey: getCollectionServiceGetCollectionQueryKey(vars.id) });
      // A smart collection's rules decide which items match, so editing them
      // changes the items list - refresh it too, not just the metadata (mirrors
      // the smart-playlist path). Without this the collection shows the old
      // matching items for up to staleTime after a rule edit.
      qc.invalidateQueries({ queryKey: getCollectionServiceGetCollectionItemsQueryKey(vars.id) });
    },
  });
}

export function useDeleteCollection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => collectionServiceDeleteCollection(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: getCollectionServiceListCollectionsQueryKey() }),
  });
}

export function addCollectionItem(collectionId: string, mediaItemId: string) {
  return collectionServiceAddCollectionItem(collectionId, { mediaItemId, sortOrder: 0 });
}


/**
 * Removes one title from a collection. Adding shipped without this for a long
 * time, so a wrong member was permanent and curation was write-only.
 */
export function useRemoveCollectionItem(collectionId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (mediaItemId: string) => collectionServiceRemoveCollectionItem(collectionId, mediaItemId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: getCollectionServiceGetCollectionItemsQueryKey(collectionId) });
      qc.invalidateQueries({ queryKey: getCollectionServiceGetCollectionQueryKey(collectionId) });
      qc.invalidateQueries({ queryKey: getCollectionServiceListCollectionsQueryKey() });
    },
  });
}

/**
 * Writes the hand-set order of a collection's members.
 *
 * The order only takes effect once the collection reads by it, so this also
 * switches sortBy to "manual" - a viewer who drags a title into place means
 * that place, not "store it and keep showing release order".
 */
export function useReorderCollectionItems(collectionId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ mediaItemIds, name }: { mediaItemIds: string[]; name: string }) => {
      await collectionServiceReorderCollectionItems(collectionId, { mediaItemIds });
      await collectionServiceUpdateCollection(collectionId, { name, sortBy: "manual" });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: getCollectionServiceGetCollectionItemsQueryKey(collectionId) });
      qc.invalidateQueries({ queryKey: getCollectionServiceGetCollectionQueryKey(collectionId) });
    },
  });
}
