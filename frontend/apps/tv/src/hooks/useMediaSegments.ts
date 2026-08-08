import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";

export interface MediaSegment {
  type: string;       // "intro" | "outro" | "recap" | "preview"
  startMs: number;
  endMs: number;
  confidence: number;
  source: string;     // "trickplay" | "audio" | "subtitle" | "chapter"
}

export interface MediaSegmentsResult {
  segments: MediaSegment[];
  // The admin "auto-skip the intro" toggle, read live per request server-side.
  // The TV app was the one player that never read it: web and mobile parse it
  // and jump client-side, and a cast receiver gets the segments withheld
  // instead, but the native TV player only ever offered the manual button - so
  // turning the toggle on did nothing here. Nil/absent = off (fail closed).
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
