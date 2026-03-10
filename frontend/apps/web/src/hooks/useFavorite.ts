import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuthStore } from "@evey/store";

const favBaseUrl = "/api/v1/favorites";

function authHeaders(): Record<string, string> {
  const token = useAuthStore.getState().accessToken;
  return token ? { Authorization: `Bearer ${token}` } : {};
}

async function fetchJson<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    ...init,
    headers: { ...authHeaders(), ...(init?.headers ?? {}) },
  });
  if (!res.ok) throw new Error(`${res.status}`);
  return res.json() as Promise<T>;
}

export function useIsFavorite(mediaId: string | undefined) {
  return useQuery({
    queryKey: ["favorite", mediaId],
    queryFn: () => fetchJson<{ isFavorite: boolean }>(`${favBaseUrl}/${mediaId}`),
    enabled: !!mediaId,
    select: (d) => d.isFavorite,
    staleTime: 30_000,
  });
}

export function useToggleFavorite(mediaId: string | undefined) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => fetchJson<{ isFavorite: boolean }>(`${favBaseUrl}/${mediaId}`, { method: "POST" }),
    onMutate: async () => {
      await qc.cancelQueries({ queryKey: ["favorite", mediaId] });
      const prev = qc.getQueryData<{ isFavorite: boolean }>(["favorite", mediaId]);
      qc.setQueryData(["favorite", mediaId], { isFavorite: !prev?.isFavorite });
      return { prev };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev) qc.setQueryData(["favorite", mediaId], ctx.prev);
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: ["favorite", mediaId] });
      qc.invalidateQueries({ queryKey: ["favorites"] });
    },
  });
}

export interface FavoriteItem {
  mediaItemId: string;
  createdAt: string;
}

export function useFavoritesList(limit = 50, offset = 0) {
  return useQuery({
    queryKey: ["favorites", limit, offset],
    queryFn: () => fetchJson<{ items: FavoriteItem[] }>(`${favBaseUrl}?limit=${limit}&offset=${offset}`),
    select: (d) => d.items ?? [],
    staleTime: 30_000,
  });
}
