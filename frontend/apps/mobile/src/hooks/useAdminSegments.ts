import { useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { useMediaSegments } from "./useMediaSegments";
import type { MediaSegment } from "./useMediaSegments";

export type { MediaSegment };

const ALL_ADMIN_TYPES = "intro,recap,end_credits,studio_logo";

export function useAdminMediaSegments(mediaId: string | undefined) {
  // Unwrap to the plain segments array: the admin editor predates the
  // introAutoSkip envelope and only ever cared about the data.
  const q = useMediaSegments(mediaId, ALL_ADMIN_TYPES.split(","));
  return { ...q, data: q.data?.segments };
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

function segmentsQueryKey(mediaId: string) {
  return ["/api/v1/media/segments", mediaId, ALL_ADMIN_TYPES];
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
      qc.invalidateQueries({ queryKey: segmentsQueryKey(mediaId) });
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
      qc.invalidateQueries({ queryKey: segmentsQueryKey(mediaId) });
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
