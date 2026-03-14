import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";

const qKey = (mediaId: string | undefined) => ["star-rating", mediaId];

export function useStarRating(mediaId: string | undefined) {
  const qc = useQueryClient();

  const { data } = useQuery({
    queryKey: qKey(mediaId),
    queryFn: () =>
      eveyFetch<{ value: number }>({ url: `/api/v1/media/${mediaId}/rating`, method: "GET" }),
    enabled: !!mediaId,
    staleTime: 60_000,
    select: (d) => d.value,
  });

  const rating = data ?? 0;

  const mutation = useMutation({
    mutationFn: async (value: number) => {
      if (value === 0) {
        await eveyFetch({ url: `/api/v1/media/${mediaId}/rating`, method: "DELETE" });
        return { value: 0 };
      }
      return eveyFetch<{ value: number }>({
        url: `/api/v1/media/${mediaId}/rating`,
        method: "POST",
        data: { value },
      });
    },
    onMutate: async (value) => {
      await qc.cancelQueries({ queryKey: qKey(mediaId) });
      const prev = qc.getQueryData(qKey(mediaId));
      qc.setQueryData(qKey(mediaId), { value });
      return { prev };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev !== undefined) qc.setQueryData(qKey(mediaId), ctx.prev);
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: qKey(mediaId) });
      qc.invalidateQueries({ queryKey: ["star-rating-agg", mediaId] });
    },
  });

  const setRating = (value: number) => {
    // clicking current star removes the rating
    mutation.mutate(rating === value ? 0 : value);
  };

  return { rating, setRating };
}

export function useRatingAggregate(mediaId: string | undefined) {
  return useQuery({
    queryKey: ["star-rating-agg", mediaId],
    queryFn: () =>
      eveyFetch<{ average: number; count: number }>({
        url: `/api/v1/media/${mediaId}/rating/aggregate`,
        method: "GET",
      }),
    enabled: !!mediaId,
    staleTime: 60_000,
  });
}
