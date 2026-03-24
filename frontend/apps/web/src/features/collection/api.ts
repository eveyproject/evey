import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";

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
    staleTime: 5 * 60_000,
  });
}

export function useCollection(id: string, limit = 50, offset = 0) {
  return useQuery<GetCollectionResponse>({
    queryKey: ["collection", id, limit, offset],
    queryFn: () =>
      eveyFetch<GetCollectionResponse>({ url: `/api/v1/collections/${id}?limit=${limit}&offset=${offset}`, method: "GET" }),
    enabled: !!id,
    staleTime: 5 * 60_000,
  });
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

export function useCreateCollection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateCollectionInput) =>
      eveyFetch<{ id: string }>({ url: "/api/v1/collections", method: "POST", data: input }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["collections"] }),
  });
}

export function useUpdateCollection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: UpdateCollectionInput) =>
      eveyFetch<void>({ url: `/api/v1/collections/${id}`, method: "PUT", data: body }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["collections"] }),
  });
}

export function useDeleteCollection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => eveyFetch<void>({ url: `/api/v1/collections/${id}`, method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["collections"] }),
  });
}
