import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  useRatingServiceGetMyThumbRating,
  ratingServicePutThumbRating,
  getRatingServiceGetMyThumbRatingQueryKey,
} from "@evey/api";

export function useMyRating(mediaId: string | undefined) {
  return useRatingServiceGetMyThumbRating(mediaId ?? "", {
    query: { enabled: !!mediaId, select: (d) => d.value ?? 0, staleTime: 60_000 },
  });
}

export function useSetRating(mediaId: string | undefined) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (value: number) => ratingServicePutThumbRating(mediaId ?? "", { value }),
    onMutate: async (value) => {
      if (!mediaId) return { prev: undefined };
      const key = getRatingServiceGetMyThumbRatingQueryKey(mediaId);
      await qc.cancelQueries({ queryKey: key });
      const prev = qc.getQueryData<{ value?: number }>(key);
      qc.setQueryData(key, { value });
      return { prev };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev !== undefined && mediaId)
        qc.setQueryData(getRatingServiceGetMyThumbRatingQueryKey(mediaId), ctx.prev);
    },
    onSettled: () => {
      if (mediaId) qc.invalidateQueries({ queryKey: getRatingServiceGetMyThumbRatingQueryKey(mediaId) });
    },
  });
}
