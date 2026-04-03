import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { toast } from "@evey/store";

export interface Collection {
  id: string;
  tmdbId?: number;
  name: string;
  overview?: string;
  posterUrl?: string;
  backdropUrl?: string;
  rules?: { field: string; operator: string; value: string }[];
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
    queryFn: () =>
      eveyFetch<ListCollectionsResponse>({
        url: "/api/v1/collections",
        method: "GET",
      }),
    staleTime: 5 * 60_000,
  });
}

export function useCollection(id: string, limit = 50, offset = 0) {
  return useQuery<GetCollectionResponse>({
    queryKey: ["collection", id, limit, offset],
    queryFn: () =>
      eveyFetch<GetCollectionResponse>({
        url: `/api/v1/collections/${id}?limit=${limit}&offset=${offset}`,
        method: "GET",
      }),
    enabled: !!id,
    staleTime: 5 * 60_000,
  });
}

export function useCreateCollection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { name: string; description?: string }) =>
      eveyFetch<{ id: string }>({ url: "/api/v1/collections", method: "POST", data: input }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["collections"] });
      toast.success("Collection created");
    },
    onError: () => toast.error("Failed to create collection"),
  });
}

export function useUpdateCollection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string; name: string; description?: string }) =>
      eveyFetch<void>({ url: `/api/v1/collections/${id}`, method: "PUT", data: body }),
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ["collections"] });
      qc.invalidateQueries({ queryKey: ["collection", vars.id] });
      toast.success("Collection updated");
    },
    onError: () => toast.error("Failed to update collection"),
  });
}

export function useDeleteCollection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      eveyFetch<void>({ url: `/api/v1/collections/${id}`, method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["collections"] });
      toast.success("Collection deleted");
    },
    onError: () => toast.error("Failed to delete collection"),
  });
}
