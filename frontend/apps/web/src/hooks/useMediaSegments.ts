import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";

export interface MediaSegment {
  type: string;       // "intro" | "recap" | "end_credits" | "studio_logo" | "preview"
  startMs: number;
  endMs: number;
  confidence: number;
  // Server-emitted values: "mkv_chapter" | "tidb" | "introdb" | "phash" |
  // "audio_cluster" | "chromaprint_discovery" | "audio" | "blackdetect" |
  // "logo_match" | "manual" | "manual_propagated"
  source: string;
}

export interface MediaSegmentsResult {
  segments: MediaSegment[];
  // Server-wide admin toggle: when true, players auto-skip intro segments
  // instead of only offering the button.
  introAutoSkip: boolean;
}

export function useMediaSegments(mediaId: string | undefined, types?: string[]) {
  const typesParam = types?.join(",") ?? "intro,outro,recap,end_credits,studio_logo";
  return useQuery<MediaSegmentsResult>({
    queryKey: ["/api/v1/media/segments", mediaId, typesParam],
    queryFn: async () => {
      try {
        const data = await eveyFetch<{ segments: MediaSegment[]; introAutoSkip?: boolean }>({
          url: `/api/v1/media/${mediaId}/segments?types=${typesParam}`,
          method: "GET",
        });
        return { segments: data.segments ?? [], introAutoSkip: data.introAutoSkip === true };
      } catch {
        return { segments: [], introAutoSkip: false };
      }
    },
    enabled: !!mediaId,
    staleTime: Infinity,
  });
}
