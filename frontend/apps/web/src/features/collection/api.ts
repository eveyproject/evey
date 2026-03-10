import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuthStore } from "@evey/store";

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

function authHeaders(): Record<string, string> {
  const token = useAuthStore.getState().accessToken;
  return token ? { Authorization: `Bearer ${token}` } : {};
}

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    ...init,
    headers: { ...authHeaders(), "Content-Type": "application/json", ...init?.headers },
  });
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  if (res.status === 204) return {} as T;
  return res.json() as Promise<T>;
}

export function useCollections() {
  return useQuery<ListCollectionsResponse>({
    queryKey: ["collections"],
    queryFn: () => fetchJSON<ListCollectionsResponse>("/api/v1/collections"),
  });
}

export function useCollection(id: string, limit = 50, offset = 0) {
  return useQuery<GetCollectionResponse>({
    queryKey: ["collection", id, limit, offset],
    queryFn: () =>
      fetchJSON<GetCollectionResponse>(
        `/api/v1/collections/${id}?limit=${limit}&offset=${offset}`
      ),
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
    }) => fetchJSON<{ id: string }>("/api/v1/collections", { method: "POST", body: JSON.stringify(body) }),
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
    }) => fetchJSON<void>(`/api/v1/collections/${id}`, { method: "PUT", body: JSON.stringify(body) }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["collections"] });
      qc.invalidateQueries({ queryKey: ["collection"] });
    },
  });
}

export function useDeleteCollection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => fetchJSON<void>(`/api/v1/collections/${id}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["collections"] }),
  });
}
