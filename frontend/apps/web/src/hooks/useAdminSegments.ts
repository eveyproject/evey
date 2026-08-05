import { useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { useMediaSegments } from "./useMediaSegments";
import type { MediaSegment } from "./useMediaSegments";

export type { MediaSegment };

const ALL_ADMIN_TYPES = "intro,recap,end_credits,studio_logo";

export function useAdminMediaSegments(mediaId: string | undefined) {
  return useMediaSegments(mediaId, ALL_ADMIN_TYPES.split(","));
}

interface SetSegmentInput {
  type: string;
  startMs: number;
  endMs: number;
}

interface PropagateResponse {
  propagatedTo: string[];
  count: number;
}

// Invalidation must hit EVERY types-variant of the segments query, not just
// the admin one: the player caches its own variant (different `types` string)
// with staleTime Infinity, so an exact-key invalidation left the skip button
// serving the pre-edit world until a full reload.
function segmentsPrefixKey(mediaId: string) {
  return ["/api/v1/media/segments", mediaId];
}

export function useSetSegment(mediaId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: SetSegmentInput) => {
      return await eveyFetch<unknown>({
        url: `/api/v1/admin/media/${mediaId}/segments`,
        method: "PUT",
        data: input,
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: segmentsPrefixKey(mediaId) });
    },
  });
}

export function useDeleteSegment(mediaId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (type: string) => {
      return await eveyFetch<unknown>({
        url: `/api/v1/admin/media/${mediaId}/segments?type=${encodeURIComponent(type)}`,
        method: "DELETE",
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: segmentsPrefixKey(mediaId) });
    },
  });
}

export function usePropagateSegment(mediaId: string) {
  return useMutation({
    mutationFn: async (type: string) => {
      return await eveyFetch<PropagateResponse>({
        url: `/api/v1/admin/media/${mediaId}/segments/propagate?type=${encodeURIComponent(type)}`,
        method: "POST",
      });
    },
  });
}

export function useTriggerAutoDetect(mediaId: string) {
  return useMutation({
    mutationFn: async () => {
      return await eveyFetch<unknown>({
        url: `/api/v1/admin/media/${mediaId}/detect-segments`,
        method: "POST",
      });
    },
  });
}
