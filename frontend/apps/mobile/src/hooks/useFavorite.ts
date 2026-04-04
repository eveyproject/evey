import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";

const favBaseUrl = "/api/v1/favorites";

export function useIsFavorite(mediaId: string | undefined) {
  return useQuery({
    queryKey: ["favorite", mediaId],
    queryFn: () => eveyFetch<{ isFavorite: boolean }>({ url: `${favBaseUrl}/${mediaId}`, method: "GET" }),
    enabled: !!mediaId,
    select: (d) => d.isFavorite,
    staleTime: 30_000,
  });
}

export function useToggleFavorite(mediaId: string | undefined) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => eveyFetch<{ isFavorite: boolean }>({ url: `${favBaseUrl}/${mediaId}`, method: "POST" }),
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

export function useBulkFavorite() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (ids: string[]) => {
      for (const id of ids) {
        await eveyFetch<{ isFavorite: boolean }>({ url: `${favBaseUrl}/${id}`, method: "POST" });
      }
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: ["favorites"] });
    },
  });
}

export interface FavoriteItem {
  mediaItemId: string;
  createdAt: string;
  title?: string;
  type?: string;
  year?: number;
  communityRating?: number;
}

export function useFavoritesList(limit = 50, offset = 0) {
  return useQuery({
    queryKey: ["favorites", limit, offset],
    queryFn: () => eveyFetch<{ items: FavoriteItem[] }>({ url: `${favBaseUrl}?limit=${limit}&offset=${offset}`, method: "GET" }),
    select: (d) => d.items ?? [],
    staleTime: 30_000,
  });
}
