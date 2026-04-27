import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";

export interface MediaSegment {
  type: string;       // "intro" | "outro" | "recap" | "preview"
  startMs: number;
  endMs: number;
  confidence: number;
  source: string;     // "trickplay" | "audio" | "subtitle" | "chapter"
}

export function useMediaSegments(mediaId: string | undefined, types?: string[]) {
  const typesParam = types?.join(",") ?? "intro,outro";
  return useQuery<MediaSegment[]>({
    queryKey: ["/api/v1/media/segments", mediaId, typesParam],
    queryFn: async () => {
      try {
        const data = await eveyFetch<{ segments: MediaSegment[] }>({
          url: `/api/v1/media/${mediaId}/segments?types=${typesParam}`,
          method: "GET",
        });
        return data.segments ?? [];
      } catch {
        return [];
      }
    },
    enabled: !!mediaId,
    staleTime: Infinity,
  });
}
