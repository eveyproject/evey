import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  useRatingServiceGetStarRating,
  useRatingServiceGetRatingAggregate,
  ratingServicePostStarRating,
  ratingServiceDeleteStarRating,
  getRatingServiceGetStarRatingQueryKey,
  getRatingServiceGetRatingAggregateQueryKey,
} from "@evey/api";

export function useStarRating(mediaId: string | undefined) {
  const qc = useQueryClient();

  const { data } = useRatingServiceGetStarRating(mediaId ?? "", {
    query: { enabled: !!mediaId, staleTime: 60_000, select: (d) => d.value ?? 0 },
  });

  const rating = data ?? 0;

  const mutation = useMutation({
    mutationFn: async (value: number) => {
      if (!mediaId) return { value: 0 };
      if (value === 0) {
        await ratingServiceDeleteStarRating(mediaId);
        return { value: 0 };
      }
      return ratingServicePostStarRating(mediaId, { value });
    },
    onMutate: async (value) => {
      if (!mediaId) return { prev: undefined };
      const key = getRatingServiceGetStarRatingQueryKey(mediaId);
      await qc.cancelQueries({ queryKey: key });
      const prev = qc.getQueryData(key);
      qc.setQueryData(key, { value });
      return { prev };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev !== undefined && mediaId)
        qc.setQueryData(getRatingServiceGetStarRatingQueryKey(mediaId), ctx.prev);
    },
    onSettled: () => {
      if (mediaId) {
        qc.invalidateQueries({ queryKey: getRatingServiceGetStarRatingQueryKey(mediaId) });
        qc.invalidateQueries({ queryKey: getRatingServiceGetRatingAggregateQueryKey(mediaId) });
      }
    },
  });

  const setRating = (value: number) => {
    mutation.mutate(rating === value ? 0 : value);
  };

  return { rating, setRating };
}

export function useRatingAggregate(mediaId: string | undefined) {
  return useRatingServiceGetRatingAggregate(mediaId ?? "", {
    query: { enabled: !!mediaId, staleTime: 60_000 },
  });
}
