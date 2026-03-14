import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";

export interface CollectionRule {
  field: string;
  operator: string;
  value: string;
}

export interface Collection {
  id: string;
  name: string;
  overview?: string;
  type: "manual" | "smart";
  rules: CollectionRule[];
  sortBy: string;
  sortOrder: string;
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

interface ListCollectionsResponse {
  collections: Collection[];
}

interface GetCollectionResponse {
  collection: Collection;
  items: CollectionItem[];
  total: number;
}

export function useCollections() {
  return useQuery<ListCollectionsResponse>({
    queryKey: ["collections"],
    queryFn: () => eveyFetch<ListCollectionsResponse>({ url: "/api/v1/collections", method: "GET" }),
  });
}

export function useCollection(id: string, limit = 50, offset = 0) {
  return useQuery<GetCollectionResponse>({
    queryKey: ["collection", id, limit, offset],
    queryFn: () =>
      eveyFetch<GetCollectionResponse>({ url: `/api/v1/collections/${id}?limit=${limit}&offset=${offset}`, method: "GET" }),
    enabled: !!id,
  });
}

export function useCreateCollection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: {
      name: string;
      overview?: string;
      type: string;
      rules: CollectionRule[];
      sortBy?: string;
      sortOrder?: string;
    }) => eveyFetch<{ id: string }>({ url: "/api/v1/collections", method: "POST", data: body }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["collections"] }),
  });
}

export function useUpdateCollection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: {
      id: string;
      name: string;
      overview?: string;
      type: string;
      rules: CollectionRule[];
      sortBy?: string;
      sortOrder?: string;
    }) => eveyFetch<void>({ url: `/api/v1/collections/${id}`, method: "PUT", data: body }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["collections"] });
      qc.invalidateQueries({ queryKey: ["collection"] });
    },
  });
}

export function useDeleteCollection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => eveyFetch<void>({ url: `/api/v1/collections/${id}`, method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["collections"] }),
  });
}

export function useAddToCollection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ collectionId, mediaItemId }: { collectionId: string; mediaItemId: string }) =>
      eveyFetch<void>({ url: `/api/v1/collections/${collectionId}/items`, method: "POST", data: { mediaItemId } }),
    onSuccess: (_data, { collectionId }) => {
      qc.invalidateQueries({ queryKey: ["collection", collectionId] });
    },
  });
}
