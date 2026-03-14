import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";

// value: -1=dislike, 0=neutral/removed, 1=like
export function useMyRating(mediaId: string | undefined) {
  return useQuery({
    queryKey: ["rating", mediaId],
    queryFn: () => eveyFetch<{ value: number }>({ url: `/api/v1/media/${mediaId}/my-rating`, method: "GET" }),
    enabled: !!mediaId,
    select: (d) => d.value,
    staleTime: 60_000,
  });
}

export function useSetRating(mediaId: string | undefined) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (value: number) =>
      eveyFetch<{ value: number }>({ url: `/api/v1/media/${mediaId}/rate`, method: "PUT", data: { value } }),
    onMutate: async (value) => {
      await qc.cancelQueries({ queryKey: ["rating", mediaId] });
      const prev = qc.getQueryData<{ value: number }>(["rating", mediaId]);
      qc.setQueryData(["rating", mediaId], { value });
      return { prev };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev !== undefined) qc.setQueryData(["rating", mediaId], ctx.prev);
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: ["rating", mediaId] });
    },
  });
}
