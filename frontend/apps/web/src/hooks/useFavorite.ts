import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  useFavoriteServiceIsFavorite,
  useFavoriteServiceListFavorites,
  favoriteServiceAddFavorite,
  favoriteServiceRemoveFavorite,
  getFavoriteServiceListFavoritesQueryKey,
  getFavoriteServiceIsFavoriteQueryKey,
} from "@evey/api";

export function useIsFavorite(mediaId: string | undefined) {
  return useFavoriteServiceIsFavorite(mediaId ?? "", undefined, {
    query: { enabled: !!mediaId, select: (d) => d.isFavorite ?? false, staleTime: 30_000 },
  });
}

export function useToggleFavorite(mediaId: string | undefined) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (prevIsFavorite: boolean) => {
      if (!mediaId) return { isFavorite: false };
      if (prevIsFavorite) {
        await favoriteServiceRemoveFavorite(mediaId);
        return { isFavorite: false };
      }
      const res = await favoriteServiceAddFavorite(mediaId, {});
      return { isFavorite: res.isFavorite ?? true };
    },
    onMutate: async (prevIsFavorite: boolean) => {
      if (!mediaId) return { prev: undefined };
      const key = getFavoriteServiceIsFavoriteQueryKey(mediaId);
      await qc.cancelQueries({ queryKey: key });
      const prev = qc.getQueryData<{ isFavorite?: boolean }>(key);
      qc.setQueryData(key, { isFavorite: !prevIsFavorite });
      return { prev };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev && mediaId) qc.setQueryData(getFavoriteServiceIsFavoriteQueryKey(mediaId), ctx.prev);
    },
    onSettled: () => {
      if (mediaId) qc.invalidateQueries({ queryKey: getFavoriteServiceIsFavoriteQueryKey(mediaId) });
      qc.invalidateQueries({ queryKey: getFavoriteServiceListFavoritesQueryKey() });
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

export function useFavoritesList() {
  return useFavoriteServiceListFavorites(undefined, {
    query: {
      select: (d) =>
        (d.items ?? []).map<FavoriteItem>((f) => ({
          mediaItemId: f.mediaItemId ?? "",
          createdAt: f.createdAt ?? "",
          title: f.title,
          type: f.type,
          year: f.year,
          communityRating: f.communityRating,
        })),
      staleTime: 30_000,
    },
  });
}
