import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";

export interface Chapter {
  title: string;
  startTimeMs: number;
  endTimeMs: number;
}

export function useChapters(mediaId: string | undefined) {
  return useQuery<Chapter[]>({
    queryKey: ["/api/v1/media/chapters", mediaId],
    queryFn: async () => {
      try {
        const data = await eveyFetch<{ chapters: Chapter[] }>({ url: `/api/v1/media/${mediaId}/chapters`, method: "GET" });
        return data.chapters ?? [];
      } catch {
        return [];
      }
    },
    enabled: !!mediaId,
    staleTime: Infinity,
  });
}
