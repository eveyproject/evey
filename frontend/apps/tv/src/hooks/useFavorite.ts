import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch, collectAllPages, EXPORT_PAGE_SIZE } from "@evey/api";

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

export interface FavoriteItem {
  mediaItemId: string;
  createdAt: string;
  title?: string;
  type?: string;
  year?: number;
  communityRating?: number;
}

// The screen sorts and filters this list on the device, so a partial list is a
// wrong answer rather than a shorter one. It used to ask for a single page of
// 200 and had no way to know it had been cut: ListFavoritesResponse carries
// only `items` - no total, no cursor. Walk the pages with the shared collector,
// which stops on a short page and is bounded by its own ceiling.
export function useFavoritesList() {
  return useQuery({
    queryKey: ["favorites"],
    queryFn: async () => {
      const collected = await collectAllPages<FavoriteItem>((pageIndex) =>
        eveyFetch<{ items: FavoriteItem[] }>({
          url: `${favBaseUrl}?limit=${EXPORT_PAGE_SIZE}&offset=${pageIndex * EXPORT_PAGE_SIZE}`,
          method: "GET",
        }).then((r) => ({ rows: r.items ?? [] })),
      );
      return collected.rows;
    },
    staleTime: 30_000,
  });
}
