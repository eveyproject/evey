import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  useRatingServiceGetMyThumbRating,
  ratingServicePutThumbRating,
  getRatingServiceGetMyThumbRatingQueryKey,
} from "@evey/api";

// Mirrors apps/web/src/hooks/useRating.ts (like=1, dislike=-1, 0=cleared),
// kept to the title page, its only user.
export function useMyThumbRating(mediaId: string | undefined) {
  return useRatingServiceGetMyThumbRating(mediaId ?? "", {
    query: { enabled: !!mediaId, select: (d) => d.value ?? 0, staleTime: 60_000 },
  });
}

export function useSetThumbRating(mediaId: string | undefined) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (value: number) => ratingServicePutThumbRating(mediaId ?? "", { value }),
    onMutate: async (value: number) => {
      if (!mediaId) return { prev: undefined };
      const key = getRatingServiceGetMyThumbRatingQueryKey(mediaId);
      await qc.cancelQueries({ queryKey: key });
      const prev = qc.getQueryData<{ value?: number }>(key);
      qc.setQueryData(key, { value });
      return { prev };
    },
    onError: (_err: Error, _vars: number, ctx?: { prev?: { value?: number } }) => {
      if (ctx?.prev !== undefined && mediaId)
        qc.setQueryData(getRatingServiceGetMyThumbRatingQueryKey(mediaId), ctx.prev);
    },
    onSettled: () => {
      if (mediaId) void qc.invalidateQueries({ queryKey: getRatingServiceGetMyThumbRatingQueryKey(mediaId) });
    },
  });
}
