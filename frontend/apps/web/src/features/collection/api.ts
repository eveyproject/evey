import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";

export interface Collection {
  id: string;
  tmdbId?: number;
  name: string;
  overview?: string;
  posterUrl?: string;
  backdropUrl?: string;
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

export function useDeleteCollection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => eveyFetch<void>({ url: `/api/v1/collections/${id}`, method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["collections"] }),
  });
}
