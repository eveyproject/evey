import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  useFavoriteServiceIsFavorite,
  favoriteServiceAddFavorite,
  favoriteServiceListFavorites,
  favoriteServiceRemoveFavorite,
  getFavoriteServiceListFavoritesQueryKey,
  getFavoriteServiceIsFavoriteQueryKey,
  collectAllPages,
  EXPORT_PAGE_SIZE,
  type V1FavoriteProto,
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

// The favourites page sorts, filters and searches over this list in the
// browser, so a partial list is not a shorter page - it is a wrong answer:
// searching for a favourite the server holds returns "no results". The hook
// used to send no pagination at all, which lands on the handler's default of
// 50 (clampLimitOffset, grpcapi/favorite_handler.go), and ListFavoritesResponse
// carries only `items` - no total, no cursor - so neither the screen nor the
// user could tell the list had been cut. Walk the pages instead, with the
// shared collector that stops on a short page and is bounded by its ceiling.
export function useFavoritesList() {
  return useQuery({
    // Deliberately the generated key: several screens already invalidate it
    // after toggling a favourite, and they must keep hitting this query.
    queryKey: getFavoriteServiceListFavoritesQueryKey(),
    staleTime: 30_000,
    queryFn: async ({ signal }) => {
      const collected = await collectAllPages<V1FavoriteProto>(
        (pageIndex) =>
          favoriteServiceListFavorites(
            { limit: EXPORT_PAGE_SIZE, offset: pageIndex * EXPORT_PAGE_SIZE },
            signal,
          ).then((r) => ({ rows: r.items ?? [] })),
      );
      return collected.rows.map<FavoriteItem>((f) => ({
        mediaItemId: f.mediaItemId ?? "",
        createdAt: f.createdAt ?? "",
        title: f.title,
        type: f.type,
        year: f.year,
        communityRating: f.communityRating,
      }));
    },
  });
}
