import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  eveyFetch,
  useCollectionServiceListCollections,
  useCollectionServiceGetCollection,
  useCollectionServiceGetCollectionItems,
  collectionServiceDeleteCollection,
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
    createdAt: c.createdAt ?? "",
    updatedAt: c.updatedAt ?? "",
  };
}

export function useCollections() {
  return useCollectionServiceListCollections({
    query: {
      select: (d) => ({ collections: (d.collections ?? []).map(mapCollection) }),
      staleTime: 5 * 60_000,
    },
  });
}

export function useCollection(id: string, limit = 50, offset = 0) {
  const head = useCollectionServiceGetCollection(id, {
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
  rules?: CollectionRule[];
  sortBy?: string;
}

interface UpdateCollectionInput {
  id: string;
  name: string;
  rules?: CollectionRule[];
  sortBy?: string;
}

// Create/Update use eveyFetch — no proto equivalent (rules/sortBy not in proto)
export function useCreateCollection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateCollectionInput) =>
      eveyFetch<{ id: string }>({ url: "/api/v1/collections", method: "POST", data: input }),
    onSuccess: () => qc.invalidateQueries({ queryKey: getCollectionServiceListCollectionsQueryKey() }),
  });
}

export function useUpdateCollection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: UpdateCollectionInput) =>
      eveyFetch<void>({ url: `/api/v1/collections/${id}`, method: "PUT", data: body }),
    onSuccess: (_d, vars) => {
      qc.invalidateQueries({ queryKey: getCollectionServiceListCollectionsQueryKey() });
      qc.invalidateQueries({ queryKey: getCollectionServiceGetCollectionQueryKey(vars.id) });
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

export { getCollectionServiceGetCollectionItemsQueryKey };
